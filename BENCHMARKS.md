# Cacheity Performance Benchmarks

This document describes how to use the benchmark suite to measure and compare performance of the caching implementation.

## Running Benchmarks

### Run All Benchmarks

```bash
go test -bench=. -benchmem -benchtime=5s
```

### Run Specific Benchmark

```bash
go test -bench=BenchmarkCacheHit -benchmem -benchtime=5s
```

### Generate CPU Profile

```bash
go test -bench=BenchmarkCacheHit -cpuprofile=cpu.prof -benchmem
go tool pprof cpu.prof
```

### Generate Memory Profile

```bash
go test -bench=BenchmarkCacheHit -memprofile=mem.prof -benchmem
go tool pprof mem.prof
```

### Compare Before/After Performance

```bash
# Baseline (current implementation)
go test -bench=. -benchmem -benchtime=5s > baseline.txt

# After changes
go test -bench=. -benchmem -benchtime=5s > optimized.txt

# Compare results
benchstat baseline.txt optimized.txt
```

Note: Install benchstat with: `go install golang.org/x/perf/cmd/benchstat@latest`

## Benchmark Categories

### 1. Cache Hit Benchmarks

**BenchmarkCacheHit**: Measures performance when serving from cache (already cached response)
- Tests deserialization overhead
- File I/O read performance
- JSON unmarshaling performance

**BenchmarkCacheHit_VariousBodySizes**: Tests cache hits with different response body sizes
- 1KB, 10KB, 100KB, 1MB
- Shows how performance scales with body size
- Highlights JSON deserialization overhead

### 2. Cache Miss Benchmarks

**BenchmarkCacheMiss**: Measures performance for first request (cache miss + backend call + cache write)
- Backend request overhead
- Serialization overhead
- File I/O write performance
- JSON marshaling performance

**BenchmarkCacheMiss_VariousBodySizes**: Tests cache misses with different response body sizes
- Same sizes as cache hit tests
- Shows serialization performance impact

### 3. Concurrency Benchmarks

**BenchmarkConcurrentCacheHit**: Tests parallel cache hit performance
- Uses `b.RunParallel()` for concurrent access
- Tests lock contention on read operations
- Validates scalability under load

**BenchmarkConcurrentCacheMiss**: Tests parallel cache miss performance
- Tests write lock contention
- Shows performance under concurrent writes to different keys

### 4. Component-Level Benchmarks

**BenchmarkFileIO**: Tests raw file I/O performance
- Write and read operations
- Baseline for file system performance
- Helps identify if bottleneck is I/O or serialization

**BenchmarkCacheKeyGeneration**: Tests cache key generation performance
- Simple paths
- Paths with query parameters
- Many query parameters (sorting overhead)

### 5. Real-World Scenarios

**BenchmarkRealWorldScenario**: Simulates realistic mixed workload
- 90% cache hits, 10% cache misses
- Multiple different URLs
- Concurrent access patterns
- 10KB response bodies

**BenchmarkEndToEnd**: Complete HTTP request/response cycle
- Uses actual HTTP server and client
- Includes network stack overhead
- Most realistic performance measure

## Interpreting Results

### Key Metrics

- **ns/op**: Nanoseconds per operation (lower is better)
- **B/op**: Bytes allocated per operation (lower is better)
- **allocs/op**: Number of allocations per operation (lower is better)
- **MB/s**: Megabytes per second throughput (higher is better)

### Example Output

```
BenchmarkCacheHit-8                    50000    35421 ns/op    4096 B/op    12 allocs/op
BenchmarkCacheMiss-8                   20000    95432 ns/op    8192 B/op    28 allocs/op
BenchmarkConcurrentCacheHit-8         200000    12345 ns/op    2048 B/op     8 allocs/op
```

### What to Look For

1. **High allocations/op**: Indicates memory pressure and GC overhead
2. **Large B/op**: Shows memory consumption per request
3. **Slow ns/op**: Identifies performance bottlenecks
4. **Poor scaling**: Compare sequential vs parallel benchmarks

## Performance Goals for Optimization

When implementing the split metadata/body storage optimization, aim for:

### Cache Hit Performance
- **Reduce allocations**: Body streaming should avoid loading entire body into memory
- **Improve throughput**: Expect 2-3x improvement for large responses (>100KB)
- **Lower memory**: Should see 50-70% reduction in B/op for cached responses

### Cache Miss Performance
- **Comparable or better**: Should not significantly degrade write performance
- **Same or fewer allocations**: Splitting files shouldn't add overhead

### Concurrent Performance
- **Better scalability**: Reduced lock contention for large responses
- **Higher throughput**: Parallel requests should scale better

## Expected Bottlenecks in Current Implementation

Based on the current JSON-based storage:

1. **JSON Serialization/Deserialization**: Major bottleneck for large responses
   - JSON encoding is CPU-intensive
   - Must allocate memory for entire response body

2. **Memory Allocations**: High memory pressure
   - Multiple copies of response body (backend, JSON, cache)
   - Large byte slices for JSON marshaling

3. **Lock Contention**: Per-key file locks
   - Reading entire file for large responses holds lock longer
   - Prevents concurrent reads of same cached response

4. **Disk I/O**: Single large file I/O operations
   - No streaming, must read/write entire response at once
   - Buffering happens in memory

## Validating Optimizations

After implementing metadata/body split:

1. **Run full benchmark suite**:
   ```bash
   go test -bench=. -benchmem -benchtime=5s > optimized.txt
   benchstat baseline.txt optimized.txt
   ```

2. **Check specific improvements**:
   - Cache hits with large bodies should be significantly faster
   - Memory allocations should be much lower
   - Concurrent performance should scale better

3. **Verify correctness**:
   ```bash
   go test -v ./...
   ```

4. **Profile hot paths**:
   ```bash
   go test -bench=BenchmarkCacheHit -cpuprofile=cpu.prof
   go tool pprof -top cpu.prof
   ```

## Continuous Benchmarking

Consider adding benchmarks to CI/CD:

```bash
# Run benchmarks and check for regressions
go test -bench=. -benchmem -benchtime=3s | tee current.txt
benchstat baseline.txt current.txt

# Fail if performance degrades >10%
# (requires custom tooling or benchstat analysis)
```

## Additional Metrics to Consider

For production monitoring, also track:
- Cache hit rate (%)
- P50, P95, P99 latencies
- Disk space usage
- Memory usage over time
- Vacuum performance (cleanup time)

## Next Steps

1. Run baseline benchmarks with current implementation
2. Save results: `go test -bench=. -benchmem -benchtime=5s > baseline.txt`
3. Implement metadata/body split optimization
4. Run benchmarks again and compare with `benchstat`
5. Profile and iterate on hot paths
