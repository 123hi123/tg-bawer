package bot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatSize_MB(t *testing.T) {
	// 1 MB
	got := formatSize(1024 * 1024)
	if got != "1.00 MB" {
		t.Fatalf("expected 1.00 MB, got %s", got)
	}
}

func TestFormatSize_GB(t *testing.T) {
	// 1.5 GB
	got := formatSize(1024 * 1024 * 1024 * 3 / 2)
	if got != "1.50 GB" {
		t.Fatalf("expected 1.50 GB, got %s", got)
	}
}

func TestFormatSize_SmallMB(t *testing.T) {
	// 512 KB = 0.50 MB
	got := formatSize(512 * 1024)
	if got != "0.50 MB" {
		t.Fatalf("expected 0.50 MB, got %s", got)
	}
}

func TestFormatSize_Zero(t *testing.T) {
	got := formatSize(0)
	if got != "0.00 MB" {
		t.Fatalf("expected 0.00 MB, got %s", got)
	}
}

func TestWalkDirSize(t *testing.T) {
	dir := t.TempDir()

	// Create test files
	os.WriteFile(filepath.Join(dir, "test.db"), make([]byte, 1024), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), make([]byte, 512), 0644)
	os.WriteFile(filepath.Join(dir, "image.png"), make([]byte, 2048), 0644)

	total, extSizes := walkDirSize(dir)

	if total != 1024+512+2048 {
		t.Fatalf("expected total 3584, got %d", total)
	}
	if extSizes[".db"] != 1024 {
		t.Fatalf("expected .db 1024, got %d", extSizes[".db"])
	}
	if extSizes[".md"] != 512 {
		t.Fatalf("expected .md 512, got %d", extSizes[".md"])
	}
	if extSizes[".png"] != 2048 {
		t.Fatalf("expected .png 2048, got %d", extSizes[".png"])
	}
}

func TestWalkDirSize_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	total, extSizes := walkDirSize(dir)

	if total != 0 {
		t.Fatalf("expected total 0, got %d", total)
	}
	if len(extSizes) != 0 {
		t.Fatalf("expected no extensions, got %v", extSizes)
	}
}
