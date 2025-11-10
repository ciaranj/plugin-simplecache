package cacheify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// createTestCache creates a cache instance for benchmarking
func createTestCache(b *testing.B, path string) http.Handler {
	b.Helper()

	cfg := &Config{
		Path:              path,
		MaxExpiry:         300,
		Cleanup:           300,
		AddStatusHeader:   true,
		QueryInKey:        false,
		MaxHeaderPairs:    3,
		MaxHeaderKeyLen:   30,
		MaxHeaderValueLen: 100,
		UpdateTimeout:     30, // 30 seconds timeout for waiting on concurrent cache updates
	}

	// Create a simple backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.WriteHeader(http.StatusOK)

		// Get body size from header if present
		bodySize := 1024 // default 1KB
		if size := r.Header.Get("X-Bench-Body-Size"); size != "" {
			fmt.Sscanf(size, "%d", &bodySize)
		}

		data := make([]byte, bodySize)
		for i := range data {
			data[i] = byte(i % 256)
		}
		_, _ = w.Write(data)
	})

	handler, err := New(context.Background(), backend, cfg, "benchmark")
	if err != nil {
		b.Fatalf("Failed to create cache: %v", err)
	}

	return handler
}

// BenchmarkCacheMiss benchmarks cache miss scenarios (first request)
func BenchmarkCacheMiss(b *testing.B) {
	tmpDir := b.TempDir()
	cache := createTestCache(b, tmpDir)

	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Bench-Body-Size", "1024")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Use unique URL for each iteration to ensure cache miss
		req.URL.Path = fmt.Sprintf("/test-%d", i)
		w := httptest.NewRecorder()
		cache.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", w.Code)
		}
	}
}

// BenchmarkCacheHit benchmarks cache hit scenarios (subsequent requests)
func BenchmarkCacheHit(b *testing.B) {
	tmpDir := b.TempDir()
	cache := createTestCache(b, tmpDir)

	// Prime the cache
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Bench-Body-Size", "1024")
	w := httptest.NewRecorder()
	cache.ServeHTTP(w, req)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		cache.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("Expected status 200, got %d", w.Code)
		}
		if w.Header().Get("Cache-Status") != "hit" {
			b.Fatalf("Expected cache hit, got %s", w.Header().Get("Cache-Status"))
		}
	}
}

// BenchmarkCacheHit_VariousBodySizes benchmarks cache hits with different body sizes
func BenchmarkCacheHit_VariousBodySizes(b *testing.B) {
	sizes := []int{
		1024,        // 1KB
		10 * 1024,   // 10KB
		100 * 1024,  // 100KB
		1024 * 1024, // 1MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%dKB", size/1024), func(b *testing.B) {
			tmpDir := b.TempDir()
			cache := createTestCache(b, tmpDir)

			// Prime the cache
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.Header.Set("X-Bench-Body-Size", fmt.Sprintf("%d", size))
			w := httptest.NewRecorder()
			cache.ServeHTTP(w, req)

			b.SetBytes(int64(size))
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				cache.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkCacheMiss_VariousBodySizes benchmarks cache misses with different body sizes
func BenchmarkCacheMiss_VariousBodySizes(b *testing.B) {
	sizes := []int{
		1024,        // 1KB
		10 * 1024,   // 10KB
		100 * 1024,  // 100KB
		1024 * 1024, // 1MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%dKB", size/1024), func(b *testing.B) {
			tmpDir := b.TempDir()
			cache := createTestCache(b, tmpDir)

			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.Header.Set("X-Bench-Body-Size", fmt.Sprintf("%d", size))

			b.SetBytes(int64(size))
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Use unique URL for each iteration to ensure cache miss
				req.URL.Path = fmt.Sprintf("/test-%d", i)
				w := httptest.NewRecorder()
				cache.ServeHTTP(w, req)
			}
		})
	}
}

// BenchmarkConcurrentCacheHit benchmarks concurrent cache hits
func BenchmarkConcurrentCacheHit(b *testing.B) {
	tmpDir := b.TempDir()
	cache := createTestCache(b, tmpDir)

	// Prime the cache
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	req.Header.Set("X-Bench-Body-Size", "1024")
	w := httptest.NewRecorder()
	cache.ServeHTTP(w, req)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		for pb.Next() {
			w := httptest.NewRecorder()
			cache.ServeHTTP(w, req)
		}
	})
}

// BenchmarkConcurrentCacheMiss benchmarks concurrent cache misses
func BenchmarkConcurrentCacheMiss(b *testing.B) {
	tmpDir := b.TempDir()
	cache := createTestCache(b, tmpDir)

	b.ResetTimer()
	b.ReportAllocs()

	var counter int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Use unique URL for each iteration to ensure cache miss
			i := atomic.AddInt64(&counter, 1)
			req := httptest.NewRequest("GET", fmt.Sprintf("http://example.com/test-%d", i), nil)
			req.Header.Set("X-Bench-Body-Size", "1024")
			w := httptest.NewRecorder()
			cache.ServeHTTP(w, req)
		}
	})
}

// BenchmarkFileIO benchmarks direct file I/O operations
func BenchmarkFileIO(b *testing.B) {
	sizes := []int{
		1024,        // 1KB
		10 * 1024,   // 10KB
		100 * 1024,  // 100KB
		1024 * 1024, // 1MB
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("write_%dKB", size/1024), func(b *testing.B) {
			tmpDir := b.TempDir()
			data := make([]byte, size)

			b.SetBytes(int64(size))
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				path := filepath.Join(tmpDir, fmt.Sprintf("file-%d", i))
				err := os.WriteFile(path, data, 0600)
				if err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("read_%dKB", size/1024), func(b *testing.B) {
			tmpDir := b.TempDir()
			data := make([]byte, size)
			path := filepath.Join(tmpDir, "file")
			err := os.WriteFile(path, data, 0600)
			if err != nil {
				b.Fatal(err)
			}

			b.SetBytes(int64(size))
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_, err := os.ReadFile(path)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCacheKeyGeneration benchmarks cache key generation
func BenchmarkCacheKeyGeneration(b *testing.B) {
	b.Run("simple_path", func(b *testing.B) {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = cacheKey(req, false)
		}
	})

	b.Run("with_query_params", func(b *testing.B) {
		req := httptest.NewRequest("GET", "http://example.com/test?foo=bar&baz=qux&abc=123", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = cacheKey(req, true)
		}
	})

	b.Run("with_many_query_params", func(b *testing.B) {
		req := httptest.NewRequest("GET", "http://example.com/test?a=1&b=2&c=3&d=4&e=5&f=6&g=7&h=8&i=9&j=10", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = cacheKey(req, true)
		}
	})
}

// BenchmarkRealWorldScenario simulates a realistic mixed workload
func BenchmarkRealWorldScenario(b *testing.B) {
	tmpDir := b.TempDir()
	cache := createTestCache(b, tmpDir)

	// Prime cache with 10 different URLs
	urls := make([]*http.Request, 10)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://example.com/page-%d", i), nil)
		req.Header.Set("X-Bench-Body-Size", "10240") // 10KB responses
		urls[i] = req

		// Prime the cache
		w := httptest.NewRecorder()
		cache.ServeHTTP(w, req)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Simulate 90% cache hits, 10% cache misses
	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			counter++
			var req *http.Request

			if counter%10 == 0 {
				// 10% cache miss - new URL
				req = httptest.NewRequest("GET", fmt.Sprintf("http://example.com/new-%d", counter), nil)
				req.Header.Set("X-Bench-Body-Size", "10240")
			} else {
				// 90% cache hit - existing URL
				req = urls[counter%len(urls)]
			}

			w := httptest.NewRecorder()
			cache.ServeHTTP(w, req)
		}
	})
}

// BenchmarkEndToEnd measures complete request/response cycle
func BenchmarkEndToEnd(b *testing.B) {
	tmpDir := b.TempDir()
	cache := createTestCache(b, tmpDir)

	// Create test server
	server := httptest.NewServer(cache)
	defer server.Close()

	client := server.Client()
	client.Timeout = 10 * time.Second

	b.Run("first_request", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			resp, err := client.Get(server.URL + fmt.Sprintf("/test-%d", i))
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	})

	b.Run("cached_request", func(b *testing.B) {
		// Prime the cache
		resp, err := client.Get(server.URL + "/cached-test")
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			resp, err := client.Get(server.URL + "/cached-test")
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	})
}
