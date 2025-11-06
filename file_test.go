package cacheify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testCacheKey = "GETlocalhost:8080/test/path"

func TestFileCache(t *testing.T) {
	dir := createTempDir(t)

	fc, err := newFileCache(dir, time.Second, 255, 100, 8192)
	if err != nil {
		t.Errorf("unexpected newFileCache error: %v", err)
	}

	_, err = fc.GetStream(testCacheKey)
	if err == nil {
		t.Error("unexpected cache content")
	}

	cacheContent := []byte("some random cache content that should be exact")
	metadata := cacheMetadata{
		Status: 200,
		Headers: map[string][]string{
			"Content-Type": {"text/plain"},
		},
	}

	writer, err := fc.SetStream(testCacheKey, metadata, time.Second)
	if err != nil {
		t.Errorf("unexpected cache set error: %v", err)
	}
	if _, err := writer.Write(cacheContent); err != nil {
		t.Errorf("unexpected write error: %v", err)
	}
	if err := writer.Commit(); err != nil {
		t.Errorf("unexpected commit error: %v", err)
	}

	resp, err := fc.GetStream(testCacheKey)
	if err != nil {
		t.Errorf("unexpected cache get error: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("unexpected read error: %v", err)
	}

	if !bytes.Equal(got, cacheContent) {
		t.Errorf("unexpected cache content: want %s, got %s", cacheContent, got)
	}

	if resp.Metadata.Status != 200 {
		t.Errorf("unexpected status: want 200, got %d", resp.Metadata.Status)
	}
}

func TestFileCache_ConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			t.Fatal(r)
		}
	}()

	dir := createTempDir(t)

	fc, err := newFileCache(dir, time.Second, 255, 100, 8192)
	if err != nil {
		t.Errorf("unexpected newFileCache error: %v", err)
	}

	cacheContent := []byte("some random cache content that should be exact")
	metadata := cacheMetadata{
		Status: 200,
		Headers: map[string][]string{
			"Content-Type": {"text/plain"},
		},
	}

	var wg sync.WaitGroup

	wg.Add(2)

	go func() {
		defer wg.Done()

		for {
			resp, _ := fc.GetStream(testCacheKey)
			if resp != nil {
				got, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if !bytes.Equal(got, cacheContent) {
					panic(fmt.Errorf("unexpected cache content: want %s, got %s", cacheContent, got))
				}
			}

			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	go func() {
		defer wg.Done()

		for {
			writer, err := fc.SetStream(testCacheKey, metadata, time.Second)
			if err != nil {
				panic(fmt.Errorf("unexpected cache set error: %w", err))
			}
			if _, err := writer.Write(cacheContent); err != nil {
				panic(fmt.Errorf("unexpected write error: %w", err))
			}
			if err := writer.Commit(); err != nil {
				panic(fmt.Errorf("unexpected commit error: %w", err))
			}

			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	wg.Wait()
}

func TestPathMutex(t *testing.T) {
	pm := &pathMutex{lock: map[string]*fileLock{}}

	mu := pm.MutexAt("sometestpath")
	mu.Lock()

	var (
		wg     sync.WaitGroup
		locked uint32
	)

	wg.Add(1)

	go func() {
		defer wg.Done()

		mu := pm.MutexAt("sometestpath")
		mu.Lock()
		defer mu.Unlock()

		atomic.AddUint32(&locked, 1)
	}()

	// locked should be 0 as we already have a lock on the path.
	if atomic.LoadUint32(&locked) != 0 {
		t.Error("unexpected second lock")
	}

	mu.Unlock()

	wg.Wait()

	if l := len(pm.lock); l > 0 {
		t.Errorf("unexpected lock length: want 0, got %d", l)
	}
}

func BenchmarkFileCache_Get(b *testing.B) {
	dir := createTempDir(b)

	fc, err := newFileCache(dir, time.Minute, 255, 100, 8192)
	if err != nil {
		b.Errorf("unexpected newFileCache error: %v", err)
	}

	metadata := cacheMetadata{
		Status: 200,
		Headers: map[string][]string{
			"Content-Type": {"text/plain"},
		},
	}
	cacheContent := []byte("some random cache content that should be exact")
	writer, _ := fc.SetStream(testCacheKey, metadata, time.Minute)
	_, _ = writer.Write(cacheContent)
	_ = writer.Commit()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, _ := fc.GetStream(testCacheKey)
		if resp != nil {
			_, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
	}
}
