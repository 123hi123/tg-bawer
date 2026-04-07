package database

import "testing"

func TestRecordModelRequestAccumulatesCounts(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	if err := db.RecordModelRequest(10, "google", "gemini-3-pro-image-preview", 1, 1, 0); err != nil {
		t.Fatalf("RecordModelRequest success failed: %v", err)
	}
	if err := db.RecordModelRequest(10, "google", "gemini-3-pro-image-preview", 1, 0, 1); err != nil {
		t.Fatalf("RecordModelRequest failure failed: %v", err)
	}
	if err := db.RecordModelRequest(10, "google", "gemini-3-pro-image-preview", 1, 1, 0); err != nil {
		t.Fatalf("RecordModelRequest second success failed: %v", err)
	}

	stats, err := db.GetUserModelRequestStats(10)
	if err != nil {
		t.Fatalf("GetUserModelRequestStats failed: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat row, got %d", len(stats))
	}

	stat := stats[0]
	if stat.Provider != "google" {
		t.Fatalf("expected provider google, got %q", stat.Provider)
	}
	if stat.ModelName != "gemini-3-pro-image-preview" {
		t.Fatalf("expected model name to match, got %q", stat.ModelName)
	}
	if stat.TotalCount != 3 {
		t.Fatalf("expected total_count=3, got %d", stat.TotalCount)
	}
	if stat.SuccessCount != 2 {
		t.Fatalf("expected success_count=2, got %d", stat.SuccessCount)
	}
	if stat.FailureCount != 1 {
		t.Fatalf("expected failure_count=1, got %d", stat.FailureCount)
	}
}

func TestRecordModelRequestNormalizesEmptyValues(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	if err := db.RecordModelRequest(20, "   ", "", 1, 0, 1); err != nil {
		t.Fatalf("RecordModelRequest failed: %v", err)
	}

	stats, err := db.GetUserModelRequestStats(20)
	if err != nil {
		t.Fatalf("GetUserModelRequestStats failed: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat row, got %d", len(stats))
	}
	if stats[0].Provider != "unknown" {
		t.Fatalf("expected normalized provider unknown, got %q", stats[0].Provider)
	}
	if stats[0].ModelName != "unknown" {
		t.Fatalf("expected normalized model unknown, got %q", stats[0].ModelName)
	}
	if stats[0].FailureCount != 1 || stats[0].TotalCount != 1 {
		t.Fatalf("unexpected stat counts: %+v", stats[0])
	}
}

func TestGetModelRequestStatsSortsByFailureThenTotal(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	inputs := []struct {
		provider string
		model    string
		ownerID  int64
		total    int64
		success  int64
		failure  int64
	}{
		{"google", "model-a", 30, 1, 0, 1},
		{"google", "model-a", 30, 1, 0, 1},
		{"google", "model-a", 30, 1, 1, 0},
		{"grok", "model-b", 30, 1, 0, 1},
		{"grok", "model-b", 30, 1, 1, 0},
		{"grok", "model-b", 30, 1, 1, 0},
		{"grok", "model-b", 30, 1, 1, 0},
		{"google", "model-c", 30, 1, 0, 1},
	}

	for _, input := range inputs {
		if err := db.RecordModelRequest(input.ownerID, input.provider, input.model, input.total, input.success, input.failure); err != nil {
			t.Fatalf("RecordModelRequest(%s, %s) failed: %v", input.provider, input.model, err)
		}
	}

	stats, err := db.GetUserModelRequestStats(30)
	if err != nil {
		t.Fatalf("GetUserModelRequestStats failed: %v", err)
	}
	if len(stats) != 3 {
		t.Fatalf("expected 3 stat rows, got %d", len(stats))
	}

	if stats[0].Provider != "google" || stats[0].ModelName != "model-a" {
		t.Fatalf("expected first row to be google/model-a, got %+v", stats[0])
	}
	if stats[1].Provider != "grok" || stats[1].ModelName != "model-b" {
		t.Fatalf("expected second row to be grok/model-b, got %+v", stats[1])
	}
	if stats[2].Provider != "google" || stats[2].ModelName != "model-c" {
		t.Fatalf("expected third row to be google/model-c, got %+v", stats[2])
	}
}

func TestGetGlobalModelRequestStatsAggregatesAllOwners(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	if err := db.RecordModelRequest(1, "google", "shared-model", 1, 1, 0); err != nil {
		t.Fatalf("RecordModelRequest owner1 failed: %v", err)
	}
	if err := db.RecordModelRequest(2, "google", "shared-model", 1, 0, 1); err != nil {
		t.Fatalf("RecordModelRequest owner2 failed: %v", err)
	}

	stats, err := db.GetGlobalModelRequestStats()
	if err != nil {
		t.Fatalf("GetGlobalModelRequestStats failed: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 global row, got %d", len(stats))
	}
	if stats[0].TotalCount != 2 || stats[0].SuccessCount != 1 || stats[0].FailureCount != 1 {
		t.Fatalf("unexpected global stats: %+v", stats[0])
	}
}
