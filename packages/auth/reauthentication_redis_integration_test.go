package auth

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisReauthenticationGrantSingleUse(t *testing.T) {
	addr := os.Getenv("SEC004_REDIS_ADDR")
	if addr == "" {
		t.Skip("SEC004_REDIS_ADDR is required for the isolated Redis integration test")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("isolated Redis unavailable: %v", err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}

	store := NewRedisReauthenticationGrantStore(client, "test:sec004:reauth:")
	service, err := NewReauthenticationService(store, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	expectation := validReauthenticationExpectation()
	token, _, err := service.Issue(ctx, expectation)
	if err != nil {
		t.Fatal(err)
	}

	var successes atomic.Int32
	var replays atomic.Int32
	var failures atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := service.Consume(ctx, token, expectation)
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, ErrReauthenticationReplayed):
				replays.Add(1)
			default:
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("successful consumes = %d", successes.Load())
	}
	if replays.Load() != 23 || failures.Load() != 0 {
		t.Fatalf("replays=%d other_failures=%d", replays.Load(), failures.Load())
	}

	keys, err := client.Keys(ctx, "test:sec004:reauth:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		if value, err := client.Get(ctx, key).Result(); err == nil && value == token {
			t.Fatal("plaintext grant persisted in Redis")
		}
	}
}
