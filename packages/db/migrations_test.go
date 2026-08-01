package db

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// migrationDir returns the path to the migrations directory relative to this test file.
func migrationDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "migrations")
}

// migrationNameRegex matches the expected naming convention: NNNN_description.{up,down}.sql
var migrationNameRegex = regexp.MustCompile(`^(\d{4})_[a-z0-9_]+\.(up|down)\.sql$`)

// FND-004 retains this destructive orphan only as legacy baseline evidence.
const legacyBaselineOrphan = "0000_baseline"

func TestMigrationFilePairing(t *testing.T) {
	dir := migrationDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read migration directory: %v", err)
	}

	upFiles := map[string]bool{}
	downFiles := map[string]bool{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		if strings.HasSuffix(name, ".up.sql") {
			base := strings.TrimSuffix(name, ".up.sql")
			upFiles[base] = true
		} else if strings.HasSuffix(name, ".down.sql") {
			base := strings.TrimSuffix(name, ".down.sql")
			downFiles[base] = true
		}
	}

	// Every up must have a matching down
	for base := range upFiles {
		if !downFiles[base] {
			t.Errorf("Migration %s.up.sql has no matching .down.sql", base)
		}
	}

	// Every down except the one documented legacy artifact must match an up.
	for base := range downFiles {
		if !upFiles[base] && base != legacyBaselineOrphan {
			t.Errorf("Migration %s.down.sql has no matching .up.sql", base)
		}
	}
	if !downFiles[legacyBaselineOrphan] {
		t.Errorf("Documented legacy orphan %s.down.sql is missing", legacyBaselineOrphan)
	}

	if len(upFiles) == 0 {
		t.Fatal("No migration files found")
	}
}

func TestMigrationSequentialNumbering(t *testing.T) {
	dir := migrationDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read migration directory: %v", err)
	}

	numbersSeen := map[int]bool{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		matches := migrationNameRegex.FindStringSubmatch(name)
		if matches == nil {
			t.Errorf("Migration file %s does not match naming convention NNNN_description.up.sql", name)
			continue
		}

		num, err := strconv.Atoi(matches[1])
		if err != nil {
			t.Errorf("Failed to parse migration number from %s: %v", name, err)
			continue
		}
		numbersSeen[num] = true
	}

	if len(numbersSeen) == 0 {
		t.Fatal("No migration numbers found")
	}

	// Collect and sort numbers
	numbers := make([]int, 0, len(numbersSeen))
	for n := range numbersSeen {
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	// Verify sequential with no gaps starting from 1
	if numbers[0] != 1 {
		t.Errorf("Migration numbering should start at 1, got %d", numbers[0])
	}
	for i := 1; i < len(numbers); i++ {
		if numbers[i] != numbers[i-1]+1 {
			t.Errorf("Gap in migration numbering: %d -> %d", numbers[i-1], numbers[i])
		}
	}
}

func TestMigrationNamingConvention(t *testing.T) {
	dir := migrationDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read migration directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		if !migrationNameRegex.MatchString(name) {
			t.Errorf("Migration file %s does not match naming convention NNNN_description.{up,down}.sql", name)
		}
	}
}

func TestMigrationFilesNotEmpty(t *testing.T) {
	dir := migrationDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read migration directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			t.Errorf("Failed to get file info for %s: %v", name, err)
			continue
		}

		if info.Size() == 0 {
			t.Errorf("Migration file %s is empty", name)
		}
	}
}

func TestMigrationCount(t *testing.T) {
	dir := migrationDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read migration directory: %v", err)
	}

	upCount := 0
	downCount := 0
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".up.sql") {
			upCount++
		} else if strings.HasSuffix(name, ".down.sql") {
			downCount++
		}
	}

	if downCount != upCount+1 {
		t.Errorf("Expected paired ups plus one documented orphan: %d up migrations, %d down migrations", upCount, downCount)
	}

	t.Logf("Found %d migration pairs", upCount)
	fmt.Printf("Migration validation: %d pairs and one documented orphan verified\n", upCount)
}
