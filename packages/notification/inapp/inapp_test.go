package inapp

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
)

// mockExecer captures ExecContext calls for verification.
type mockExecer struct {
	mu      sync.Mutex
	calls   []execCall
	failAt  int // fail the Nth call (1-indexed), 0 = never fail
}

type execCall struct {
	query string
	args  []any
}

func (m *mockExecer) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, execCall{query: query, args: args})
	if m.failAt > 0 && len(m.calls) == m.failAt {
		return nil, fmt.Errorf("simulated failure")
	}
	return mockResult{}, nil
}

type mockResult struct{}

func (mockResult) LastInsertId() (int64, error) { return 0, nil }
func (mockResult) RowsAffected() (int64, error) { return 0, nil }

func TestCreateNotificationBatch_EmptyUsers(t *testing.T) {
	m := &mockExecer{}
	err := CreateNotificationBatch(context.Background(), m, nil, "test", "title", "msg", nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(m.calls) != 0 {
		t.Fatalf("expected 0 ExecContext calls, got %d", len(m.calls))
	}
}

func TestCreateNotificationBatch_SingleUser(t *testing.T) {
	m := &mockExecer{}
	err := CreateNotificationBatch(context.Background(), m, []string{"user-1"}, "test", "title", "msg", map[string]interface{}{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 1 {
		t.Fatalf("expected 1 ExecContext call, got %d", len(m.calls))
	}
	call := m.calls[0]
	if len(call.args) != 6 {
		t.Fatalf("expected 6 args, got %d", len(call.args))
	}
	// Verify parameter numbering in query
	if !strings.Contains(call.query, "($1, $2, $3, $4, $5, $6::jsonb)") {
		t.Fatalf("unexpected query: %s", call.query)
	}
	// Verify args: id(uuid), userID, type, title, message, metadataJSON
	if call.args[1] != "user-1" {
		t.Fatalf("expected userID 'user-1', got %v", call.args[1])
	}
	if call.args[2] != "test" {
		t.Fatalf("expected notifType 'test', got %v", call.args[2])
	}
}

func TestCreateNotificationBatch_FiveUsers(t *testing.T) {
	m := &mockExecer{}
	users := []string{"u1", "u2", "u3", "u4", "u5"}
	err := CreateNotificationBatch(context.Background(), m, users, "contest_starting", "Title", "Msg", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 1 {
		t.Fatalf("expected 1 ExecContext call, got %d", len(m.calls))
	}
	call := m.calls[0]
	if len(call.args) != 30 {
		t.Fatalf("expected 30 args (5 users * 6 params), got %d", len(call.args))
	}
	// Verify 5 value groups in the query
	if strings.Count(call.query, "::jsonb)") != 5 {
		t.Fatalf("expected 5 value groups, query: %s", call.query)
	}
	// Verify parameter numbering for last row: $25,$26,$27,$28,$29,$30
	if !strings.Contains(call.query, "($25, $26, $27, $28, $29, $30::jsonb)") {
		t.Fatalf("unexpected param numbering in query: %s", call.query)
	}
	// Verify each user is in the correct arg position
	for i, u := range users {
		if call.args[i*6+1] != u {
			t.Fatalf("expected user %q at args[%d], got %v", u, i*6+1, call.args[i*6+1])
		}
	}
}

func TestCreateNotificationBatch_ChunkSplit(t *testing.T) {
	m := &mockExecer{}
	users := make([]string, 501)
	for i := range users {
		users[i] = fmt.Sprintf("user-%d", i)
	}
	err := CreateNotificationBatch(context.Background(), m, users, "system", "Title", "Msg", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 2 {
		t.Fatalf("expected 2 ExecContext calls (500 + 1), got %d", len(m.calls))
	}
	// First chunk: 500 users = 3000 args
	if len(m.calls[0].args) != 3000 {
		t.Fatalf("expected 3000 args in first chunk, got %d", len(m.calls[0].args))
	}
	// Second chunk: 1 user = 6 args
	if len(m.calls[1].args) != 6 {
		t.Fatalf("expected 6 args in second chunk, got %d", len(m.calls[1].args))
	}
	// Verify second chunk starts with $1 (parameter numbering resets per chunk)
	if !strings.Contains(m.calls[1].query, "($1, $2, $3, $4, $5, $6::jsonb)") {
		t.Fatalf("second chunk should start param numbering at $1, query: %s", m.calls[1].query)
	}
}

func TestCreateNotificationBatch_PartialFailure(t *testing.T) {
	m := &mockExecer{failAt: 1}
	users := []string{"u1", "u2", "u3"}
	err := CreateNotificationBatch(context.Background(), m, users, "test", "Title", "Msg", nil)
	if err == nil {
		t.Fatal("expected error for partial failure")
	}
	expected := "failed to create 3/3 notifications"
	if err.Error() != expected {
		t.Fatalf("expected error %q, got %q", expected, err.Error())
	}
}

func TestCreateNotificationBatch_ExactChunkSize(t *testing.T) {
	m := &mockExecer{}
	users := make([]string, 500)
	for i := range users {
		users[i] = fmt.Sprintf("user-%d", i)
	}
	err := CreateNotificationBatch(context.Background(), m, users, "system", "Title", "Msg", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 1 {
		t.Fatalf("expected 1 ExecContext call for exactly 500 users, got %d", len(m.calls))
	}
	if len(m.calls[0].args) != 3000 {
		t.Fatalf("expected 3000 args, got %d", len(m.calls[0].args))
	}
}

func TestCreateNotificationBatch_Chunking(t *testing.T) {
	tests := []struct {
		name          string
		numUsers      int
		expectedCalls int
	}{
		{"1200 users = 3 chunks (500+500+200)", 1200, 3},
		{"500 users = 1 chunk", 500, 1},
		{"1 user = 1 chunk", 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockExecer{}
			users := make([]string, tt.numUsers)
			for i := range users {
				users[i] = fmt.Sprintf("user-%d", i)
			}
			err := CreateNotificationBatch(context.Background(), m, users, "system", "Title", "Msg", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(m.calls) != tt.expectedCalls {
				t.Fatalf("expected %d ExecContext calls, got %d", tt.expectedCalls, len(m.calls))
			}
		})
	}
}

func BenchmarkCreateNotificationBatch(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, n := range sizes {
		b.Run(fmt.Sprintf("users=%d", n), func(b *testing.B) {
			users := make([]string, n)
			for i := range users {
				users[i] = fmt.Sprintf("user-%d", i)
			}
			meta := map[string]interface{}{"contest_id": "c-1"}
			expectedCalls := int(math.Ceil(float64(n) / float64(batchChunkSize)))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m := &mockExecer{}
				err := CreateNotificationBatch(context.Background(), m, users, "system", "Title", "Msg", meta)
				if err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
				if len(m.calls) != expectedCalls {
					b.Fatalf("expected %d ExecContext calls for %d users, got %d", expectedCalls, n, len(m.calls))
				}
			}
		})
	}
}
