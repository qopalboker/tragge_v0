package server

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWriteAheadLog_BasicOperations(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100}, nil)

	// Test Write
	seqNum := wal.Write(WALOpCreatePosition, "contest1", "user1", "AAPL", []byte(`{"test": true}`))
	if seqNum != 1 {
		t.Errorf("Expected seq_num 1, got %d", seqNum)
	}

	// Test stats after write
	stats := wal.GetStats()
	if stats.TotalEntries != 1 {
		t.Errorf("Expected 1 entry, got %d", stats.TotalEntries)
	}
	if stats.PendingCount != 1 {
		t.Errorf("Expected 1 pending, got %d", stats.PendingCount)
	}
	if stats.CommittedCount != 0 {
		t.Errorf("Expected 0 committed, got %d", stats.CommittedCount)
	}

	// Test MarkCommitted
	if !wal.MarkCommitted(seqNum) {
		t.Error("MarkCommitted should return true for existing entry")
	}

	stats = wal.GetStats()
	if stats.PendingCount != 0 {
		t.Errorf("Expected 0 pending after commit, got %d", stats.PendingCount)
	}
	if stats.CommittedCount != 1 {
		t.Errorf("Expected 1 committed, got %d", stats.CommittedCount)
	}
}

func TestWriteAheadLog_MarkRolledBack(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100}, nil)

	seqNum := wal.Write(WALOpUpdatePosition, "contest1", "user1", "TSLA", []byte(`{}`))

	if !wal.MarkRolledBack(seqNum) {
		t.Error("MarkRolledBack should return true for existing entry")
	}

	stats := wal.GetStats()
	if stats.RolledBackCount != 1 {
		t.Errorf("Expected 1 rolled back, got %d", stats.RolledBackCount)
	}
}

func TestWriteAheadLog_RingBuffer(t *testing.T) {
	maxEntries := 10
	wal := NewWriteAheadLog(WALConfig{MaxEntries: maxEntries}, nil)

	// Write more entries than max capacity
	for i := 0; i < maxEntries+5; i++ {
		wal.Write(WALOpCreatePosition, "contest1", "user1", "SYM", []byte(`{}`))
	}

	// Ring buffer should only keep last maxEntries
	stats := wal.GetStats()
	if stats.TotalEntries != maxEntries {
		t.Errorf("Expected %d entries (ring buffer limit), got %d", maxEntries, stats.TotalEntries)
	}

	// Sequence number should be accurate
	if stats.CurrentSeqNum != uint64(maxEntries+5) {
		t.Errorf("Expected seq_num %d, got %d", maxEntries+5, stats.CurrentSeqNum)
	}
}

func TestWriteAheadLog_GetPendingEntries(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100}, nil)

	// Write some entries
	seq1 := wal.Write(WALOpCreatePosition, "c1", "u1", "AAPL", []byte(`{}`))
	seq2 := wal.Write(WALOpUpdatePosition, "c1", "u1", "GOOG", []byte(`{}`))
	seq3 := wal.Write(WALOpClosePosition, "c1", "u1", "MSFT", []byte(`{}`))

	// Mark one as committed
	wal.MarkCommitted(seq2)

	// Get pending entries
	pending := wal.GetPendingEntries()
	if len(pending) != 2 {
		t.Errorf("Expected 2 pending entries, got %d", len(pending))
	}

	// Verify only seq1 and seq3 are pending
	seqNums := make(map[uint64]bool)
	for _, e := range pending {
		seqNums[e.SeqNum] = true
	}
	if !seqNums[seq1] || !seqNums[seq3] {
		t.Error("Missing expected pending entries")
	}
	if seqNums[seq2] {
		t.Error("Committed entry should not be in pending list")
	}
}

func TestWriteAheadLog_Divergence(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100}, nil)

	if wal.IsDiverged() {
		t.Error("New WAL should not be diverged")
	}

	wal.SetDiverged()
	if !wal.IsDiverged() {
		t.Error("WAL should be diverged after SetDiverged")
	}

	isDiverged, divergedAt := wal.GetDivergenceInfo()
	if !isDiverged {
		t.Error("GetDivergenceInfo should return diverged=true")
	}
	if divergedAt.IsZero() {
		t.Error("Diverged time should not be zero")
	}

	wal.ClearDiverged()
	if wal.IsDiverged() {
		t.Error("WAL should not be diverged after ClearDiverged")
	}
}

func TestWriteAheadLog_Concurrency(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 1000}, nil)

	var wg sync.WaitGroup
	numGoroutines := 10
	numWritesPerGoroutine := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numWritesPerGoroutine; j++ {
				wal.Write(WALOpCreatePosition, "contest", "user", "SYM", []byte(`{}`))
			}
		}(i)
	}

	wg.Wait()

	stats := wal.GetStats()
	expectedTotal := numGoroutines * numWritesPerGoroutine
	if stats.CurrentSeqNum != uint64(expectedTotal) {
		t.Errorf("Expected seq_num %d after concurrent writes, got %d", expectedTotal, stats.CurrentSeqNum)
	}
}

func TestWriteAheadLog_MarkNonExistent(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100}, nil)

	// Try to mark a non-existent entry
	if wal.MarkCommitted(999) {
		t.Error("MarkCommitted should return false for non-existent entry")
	}
	if wal.MarkRolledBack(999) {
		t.Error("MarkRolledBack should return false for non-existent entry")
	}
}

func TestWriteAheadLog_EntryData(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100}, nil)

	contestID := "test-contest"
	userID := "test-user"
	symbol := "AAPL"
	data := []byte(`{"position_id": "pos-123", "qty": 100}`)

	seqNum := wal.Write(WALOpCreatePosition, contestID, userID, symbol, data)

	pending := wal.GetPendingEntries()
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending entry, got %d", len(pending))
	}

	entry := pending[0]
	if entry.SeqNum != seqNum {
		t.Errorf("SeqNum mismatch: expected %d, got %d", seqNum, entry.SeqNum)
	}
	if entry.ContestID != contestID {
		t.Errorf("ContestID mismatch: expected %s, got %s", contestID, entry.ContestID)
	}
	if entry.UserID != userID {
		t.Errorf("UserID mismatch: expected %s, got %s", userID, entry.UserID)
	}
	if entry.Symbol != symbol {
		t.Errorf("Symbol mismatch: expected %s, got %s", symbol, entry.Symbol)
	}
	if entry.Operation != WALOpCreatePosition {
		t.Errorf("Operation mismatch: expected %s, got %s", WALOpCreatePosition, entry.Operation)
	}
	if entry.Status != WALStatusPending {
		t.Errorf("Status mismatch: expected %s, got %s", WALStatusPending, entry.Status)
	}
	if string(entry.Data) != string(data) {
		t.Errorf("Data mismatch: expected %s, got %s", string(data), string(entry.Data))
	}
	if entry.Timestamp.After(time.Now()) || entry.Timestamp.Before(time.Now().Add(-time.Second)) {
		t.Error("Timestamp should be recent")
	}
}

func TestWALStats(t *testing.T) {
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100}, nil)

	// Write some entries with different statuses
	seq1 := wal.Write(WALOpCreatePosition, "c1", "u1", "AAPL", []byte(`{}`))
	seq2 := wal.Write(WALOpUpdatePosition, "c1", "u1", "GOOG", []byte(`{}`))
	_ = wal.Write(WALOpClosePosition, "c1", "u1", "MSFT", []byte(`{}`))

	wal.MarkCommitted(seq1)
	wal.MarkRolledBack(seq2)
	// seq3 stays pending

	stats := wal.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("Expected 3 total entries, got %d", stats.TotalEntries)
	}
	if stats.CommittedCount != 1 {
		t.Errorf("Expected 1 committed, got %d", stats.CommittedCount)
	}
	if stats.RolledBackCount != 1 {
		t.Errorf("Expected 1 rolled back, got %d", stats.RolledBackCount)
	}
	if stats.PendingCount != 1 {
		t.Errorf("Expected 1 pending, got %d", stats.PendingCount)
	}
	if stats.MaxEntries != 100 {
		t.Errorf("Expected max entries 100, got %d", stats.MaxEntries)
	}
	if stats.CurrentSeqNum != 3 {
		t.Errorf("Expected current seq num 3, got %d", stats.CurrentSeqNum)
	}
}

// --- File persistence tests ---

func TestWAL_FilePersistence(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.jsonl")

	// Create WAL and write some entries
	wal1 := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	wal1.Write(WALOpCreatePosition, "c1", "u1", "AAPL", []byte(`{"pos":"p1"}`))
	wal1.Write(WALOpUpdatePosition, "c1", "u1", "GOOG", []byte(`{"pos":"p2"}`))
	wal1.Close()

	// Create a new WAL from the same file — pending entries should be recovered
	wal2 := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	defer wal2.Close()

	pending := wal2.GetPendingEntries()
	if len(pending) != 2 {
		t.Fatalf("Expected 2 pending entries after recovery, got %d", len(pending))
	}

	// Verify entry data survives
	symbols := make(map[string]bool)
	for _, e := range pending {
		symbols[e.Symbol] = true
	}
	if !symbols["AAPL"] || !symbols["GOOG"] {
		t.Errorf("Recovered entries missing expected symbols, got %v", symbols)
	}

	// Sequence counter should continue from the highest seen
	stats := wal2.GetStats()
	if stats.CurrentSeqNum < 2 {
		t.Errorf("Expected seq_num >= 2 after recovery, got %d", stats.CurrentSeqNum)
	}
}

func TestWAL_FileRecoveryAfterCommit(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.jsonl")

	// Create WAL, write entries, commit all
	wal1 := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	seq1 := wal1.Write(WALOpCreatePosition, "c1", "u1", "AAPL", []byte(`{}`))
	seq2 := wal1.Write(WALOpUpdatePosition, "c1", "u1", "GOOG", []byte(`{}`))
	wal1.MarkCommitted(seq1)
	wal1.MarkCommitted(seq2)
	wal1.Close()

	// Recovery should yield zero pending entries
	wal2 := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	defer wal2.Close()

	pending := wal2.GetPendingEntries()
	if len(pending) != 0 {
		t.Fatalf("Expected 0 pending entries after full commit, got %d", len(pending))
	}
}

func TestWAL_FileRecoveryMixed(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.jsonl")

	// Write 3 entries: commit 1, rollback 1, leave 1 pending
	wal1 := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	seq1 := wal1.Write(WALOpCreatePosition, "c1", "u1", "AAPL", []byte(`{"id":"committed"}`))
	seq2 := wal1.Write(WALOpUpdatePosition, "c1", "u1", "GOOG", []byte(`{"id":"rolledback"}`))
	wal1.Write(WALOpClosePosition, "c1", "u1", "MSFT", []byte(`{"id":"pending"}`))
	wal1.MarkCommitted(seq1)
	wal1.MarkRolledBack(seq2)
	wal1.Close()

	// Recovery should only load the pending entry
	wal2 := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	defer wal2.Close()

	pending := wal2.GetPendingEntries()
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending entry, got %d", len(pending))
	}
	if pending[0].Symbol != "MSFT" {
		t.Errorf("Expected pending entry for MSFT, got %s", pending[0].Symbol)
	}
}

func TestWAL_Compact(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.jsonl")

	// Write many entries and commit most of them
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	for i := 0; i < 50; i++ {
		seq := wal.Write(WALOpCreatePosition, "c1", "u1", "SYM", []byte(`{}`))
		if i < 48 {
			wal.MarkCommitted(seq)
		}
	}

	// Compact — drains async flush channel and rewrites file with only pending entries
	if err := wal.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	// After compaction, file should contain only 2 pending entries
	info, err := os.Stat(walPath)
	if err != nil {
		t.Fatalf("Failed to stat WAL file after compact: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Expected non-empty WAL file after compaction with 2 pending entries")
	}

	// Pending entries should still be intact
	pending := wal.GetPendingEntries()
	if len(pending) != 2 {
		t.Errorf("Expected 2 pending entries after compact, got %d", len(pending))
	}

	// WAL should still be functional after compact
	newSeq := wal.Write(WALOpUpdatePosition, "c1", "u1", "NEW", []byte(`{}`))
	if newSeq == 0 {
		t.Error("WAL should still work after compaction")
	}
	wal.Close()
}

func TestWAL_BackwardCompatible(t *testing.T) {
	// Empty PersistPath should work exactly like before
	wal := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: ""}, nil)

	seq := wal.Write(WALOpCreatePosition, "c1", "u1", "AAPL", []byte(`{}`))
	if seq != 1 {
		t.Errorf("Expected seq 1, got %d", seq)
	}

	wal.MarkCommitted(seq)
	stats := wal.GetStats()
	if stats.CommittedCount != 1 {
		t.Errorf("Expected 1 committed, got %d", stats.CommittedCount)
	}

	// Compact on in-memory WAL should be a no-op
	if err := wal.Compact(); err != nil {
		t.Errorf("Compact should be no-op for in-memory WAL, got error: %v", err)
	}

	// Close should be safe
	if err := wal.Close(); err != nil {
		t.Errorf("Close should be safe for in-memory WAL, got error: %v", err)
	}
}

func TestWAL_CorruptLine(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.jsonl")

	// Write a valid WAL file with a corrupt line in the middle
	wal1 := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)
	wal1.Write(WALOpCreatePosition, "c1", "u1", "AAPL", []byte(`{"pos":"p1"}`))
	wal1.Close()

	// Append a corrupt line to the file
	f, err := os.OpenFile(walPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open WAL file: %v", err)
	}
	f.Write([]byte("this is not valid json\n"))
	f.Close()

	// Write another valid entry manually (simulate append after corruption)
	wal2temp := NewWriteAheadLog(WALConfig{MaxEntries: 100, PersistPath: walPath}, nil)

	// The WAL should recover the valid entry and skip the corrupt one
	pending := wal2temp.GetPendingEntries()
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending entry (corrupt line skipped), got %d", len(pending))
	}
	if pending[0].Symbol != "AAPL" {
		t.Errorf("Expected AAPL, got %s", pending[0].Symbol)
	}
	wal2temp.Close()
}
