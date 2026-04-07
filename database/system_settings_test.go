package database

import "testing"

func TestSystemIntSetting(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	got, err := db.GetSystemInt("retry_limit_user", 5)
	if err != nil {
		t.Fatalf("GetSystemInt default failed: %v", err)
	}
	if got != 5 {
		t.Fatalf("expected default 5, got %d", got)
	}

	if err := db.SetSystemInt("retry_limit_user", 9); err != nil {
		t.Fatalf("SetSystemInt failed: %v", err)
	}

	got, err = db.GetSystemInt("retry_limit_user", 5)
	if err != nil {
		t.Fatalf("GetSystemInt stored failed: %v", err)
	}
	if got != 9 {
		t.Fatalf("expected stored 9, got %d", got)
	}
}
