#!/bin/bash

# Benchmark script to establish baseline performance metrics
# Run this before making any optimization changes

set -e

echo "=========================================="
echo "Running Baseline Performance Benchmarks"
echo "=========================================="
echo ""

# Create results directory if it doesn't exist
mkdir -p benchmark_results

# Get timestamp for filename
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BASELINE_FILE="benchmark_results/baseline_${TIMESTAMP}.txt"

echo "Running benchmarks... (this may take several minutes)"
echo "Results will be saved to: ${BASELINE_FILE}"
echo ""

# Run all benchmarks with sufficient iterations
go test -bench=. -benchmem -benchtime=5s -timeout=30m | tee "${BASELINE_FILE}"

echo ""
echo "=========================================="
echo "Baseline benchmarks complete!"
echo "=========================================="
echo ""
echo "Results saved to: ${BASELINE_FILE}"
echo ""
echo "To compare with future optimizations:"
echo "  1. Run this script now to capture baseline"
echo "  2. Make your optimizations"
echo "  3. Run: go test -bench=. -benchmem -benchtime=5s > optimized.txt"
echo "  4. Compare: benchstat ${BASELINE_FILE} optimized.txt"
echo ""
echo "Install benchstat if needed:"
echo "  go install golang.org/x/perf/cmd/benchstat@latest"
echo ""
