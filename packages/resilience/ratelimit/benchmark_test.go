package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkUserLimiter_Allow benchmarks single Allow calls.
func BenchmarkUserLimiter_Allow(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("user-1")
	}
}

// BenchmarkUserLimiter_Allow_Parallel benchmarks concurrent Allow calls.
func BenchmarkUserLimiter_Allow_Parallel(b *testing.B) {
	cfg := Config{
		Rate:      1000000,
		Window:    time.Second,
		BurstSize: 1000000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow("user-1")
		}
	})
}

// BenchmarkUserLimiter_MultiUser benchmarks with multiple users.
func BenchmarkUserLimiter_MultiUser(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	users := make([]string, 1000)
	for i := range users {
		users[i] = fmt.Sprintf("user-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(users[i%len(users)])
	}
}

// BenchmarkUserLimiter_MultiUser_Parallel benchmarks concurrent multi-user access.
func BenchmarkUserLimiter_MultiUser_Parallel(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	users := make([]string, 1000)
	for i := range users {
		users[i] = fmt.Sprintf("user-%d", i)
	}

	var counter uint64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var local uint64
		for pb.Next() {
			limiter.Allow(users[local%uint64(len(users))])
			local++
		}
	})
	_ = counter
}

// BenchmarkWebSocketLimiter_Allow benchmarks WebSocket limiter.
func BenchmarkWebSocketLimiter_Allow(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewWebSocketLimiter(cfg)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("conn-1")
	}
}

// BenchmarkWebSocketLimiter_Allow_Parallel benchmarks concurrent WebSocket access.
func BenchmarkWebSocketLimiter_Allow_Parallel(b *testing.B) {
	cfg := Config{
		Rate:      1000000,
		Window:    time.Second,
		BurstSize: 1000000,
	}
	limiter := NewWebSocketLimiter(cfg)
	defer limiter.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow("conn-1")
		}
	})
}

// BenchmarkConnectionLimiter_Allow benchmarks single connection limiter.
func BenchmarkConnectionLimiter_Allow(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewConnectionLimiter("conn-1", cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow()
	}
}

// BenchmarkLeakyBucket_Allow benchmarks leaky bucket.
func BenchmarkLeakyBucket_Allow(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	lb := NewLeakyBucket(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.Allow("")
	}
}

// BenchmarkMultiLimiter_Allow benchmarks combined limiters.
func BenchmarkMultiLimiter_Allow(b *testing.B) {
	shortTerm := NewUserLimiter(Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	})
	defer shortTerm.Close()

	longTerm := NewUserLimiter(Config{
		Rate:      100000,
		Window:    time.Minute,
		BurstSize: 100000,
	})
	defer longTerm.Close()

	multi := NewMultiLimiter(shortTerm, longTerm)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		multi.Allow("user-1")
	}
}

// BenchmarkUserLimiter_Remaining benchmarks checking remaining.
func BenchmarkUserLimiter_Remaining(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	// Create a user bucket first
	limiter.Allow("user-1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Remaining("user-1")
	}
}

// BenchmarkUserLimiter_Check benchmarks full status check.
func BenchmarkUserLimiter_Check(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	limiter.Allow("user-1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Check("user-1")
	}
}

// BenchmarkUserLimiter_Contention measures performance under high contention.
func BenchmarkUserLimiter_Contention(b *testing.B) {
	cfg := Config{
		Rate:      1000000,
		Window:    time.Second,
		BurstSize: 1000000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	// All goroutines hit the same user
	userID := "contended-user"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow(userID)
		}
	})
}

// BenchmarkUserLimiter_NoContention measures performance with no contention.
func BenchmarkUserLimiter_NoContention(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	// Each goroutine has its own user
	var userCounter uint64
	var mu sync.Mutex

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		mu.Lock()
		userCounter++
		userID := fmt.Sprintf("user-%d", userCounter)
		mu.Unlock()

		for pb.Next() {
			limiter.Allow(userID)
		}
	})
}

// BenchmarkBurstController_Allow benchmarks burst controller.
func BenchmarkBurstController_Allow(b *testing.B) {
	shortTerm := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	longTerm := Config{
		Rate:      100000,
		Window:    time.Minute,
		BurstSize: 100000,
	}

	bc := NewBurstController(shortTerm, longTerm)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.Allow("user-1")
	}
}

// BenchmarkMessageTypeRateLimiter_Allow benchmarks message type limiter.
func BenchmarkMessageTypeRateLimiter_Allow(b *testing.B) {
	defaultLimiter := NewUserLimiter(Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	})
	defer defaultLimiter.Close()

	orderLimiter := NewUserLimiter(Config{
		Rate:      1000,
		Window:    time.Second,
		BurstSize: 1000,
	})
	defer orderLimiter.Close()

	mtl := NewMessageTypeRateLimiter(defaultLimiter)
	mtl.SetTypeLimit("order", orderLimiter)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mtl.Allow("conn-1", "order")
	}
}

// BenchmarkMetrics_Record benchmarks metrics recording.
func BenchmarkMetrics_Record(b *testing.B) {
	metrics := NewMetrics(MetricsConfig{
		Namespace: "bench",
		Subsystem: "ratelimit",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordAllowed("limiter", "user-1")
	}
}

// BenchmarkMetrics_RecordWithLimiter benchmarks limiter with metrics.
func BenchmarkMetrics_RecordWithLimiter(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	metrics := NewMetrics(MetricsConfig{
		Namespace: "bench",
		Subsystem: "ratelimit",
	})
	limiter := NewUserLimiterWithMetrics(cfg, metrics)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("user-1")
	}
}

// BenchmarkUserLimiter_ActiveUsers benchmarks counting active users.
func BenchmarkUserLimiter_ActiveUsers(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	// Create 1000 users
	for i := 0; i < 1000; i++ {
		limiter.Allow(fmt.Sprintf("user-%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.ActiveUsers()
	}
}

// BenchmarkWebSocketLimiter_ConnectionCount benchmarks counting connections.
func BenchmarkWebSocketLimiter_ConnectionCount(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewWebSocketLimiter(cfg)
	defer limiter.Close()

	// Create 1000 connections
	for i := 0; i < 1000; i++ {
		limiter.Allow(fmt.Sprintf("conn-%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.ConnectionCount()
	}
}

// BenchmarkUserLimiter_AllowN benchmarks AllowN calls.
func BenchmarkUserLimiter_AllowN(b *testing.B) {
	cfg := Config{
		Rate:      100000,
		Window:    time.Second,
		BurstSize: 100000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.AllowN("user-1", 10)
	}
}

// Memory allocation benchmarks

// BenchmarkUserLimiter_Allow_Allocs measures allocations per Allow.
func BenchmarkUserLimiter_Allow_Allocs(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	// Warm up
	limiter.Allow("user-1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("user-1")
	}
}

// BenchmarkWebSocketLimiter_Allow_Allocs measures allocations.
func BenchmarkWebSocketLimiter_Allow_Allocs(b *testing.B) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewWebSocketLimiter(cfg)
	defer limiter.Close()

	// Warm up
	limiter.Allow("conn-1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("conn-1")
	}
}

// Scalability benchmarks with varying number of users

func BenchmarkUserLimiter_Scale_10Users(b *testing.B) {
	benchmarkScale(b, 10)
}

func BenchmarkUserLimiter_Scale_100Users(b *testing.B) {
	benchmarkScale(b, 100)
}

func BenchmarkUserLimiter_Scale_1000Users(b *testing.B) {
	benchmarkScale(b, 1000)
}

func BenchmarkUserLimiter_Scale_10000Users(b *testing.B) {
	benchmarkScale(b, 10000)
}

func benchmarkScale(b *testing.B, numUsers int) {
	cfg := Config{
		Rate:      10000,
		Window:    time.Second,
		BurstSize: 10000,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	users := make([]string, numUsers)
	for i := range users {
		users[i] = fmt.Sprintf("user-%d", i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var i int
		for pb.Next() {
			limiter.Allow(users[i%len(users)])
			i++
		}
	})
}
