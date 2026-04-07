package bot

import (
	"strings"
	"testing"

	"tg-bawer/database"
)

func TestRetryLimitForUser(t *testing.T) {
	db, err := database.NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	b := &Bot{db: db, adminUserID: testAdminUserID}

	if got := b.retryLimitForUser(123); got != normalRetryLimit {
		t.Fatalf("expected normal retry limit %d, got %d", normalRetryLimit, got)
	}

	if err := db.AddVIPUser(456); err != nil {
		t.Fatalf("AddVIPUser failed: %v", err)
	}
	if got := b.retryLimitForUser(456); got != vipRetryLimit {
		t.Fatalf("expected vip retry limit %d, got %d", vipRetryLimit, got)
	}

	if got := b.retryLimitForUser(testAdminUserID); got != adminRetryLimit {
		t.Fatalf("expected admin retry limit %d, got %d", adminRetryLimit, got)
	}
}

func TestBuildRetryExhaustedMessage(t *testing.T) {
	msg := buildRetryExhaustedMessage(
		modelStatTarget{OwnerUserID: 1, Provider: "google", Model: "gemini-3-pro-image-preview"},
		"畫一隻貓",
		5,
		"quota exceeded",
	)

	if !strings.Contains(msg, "gemini-3-pro-image-preview") {
		t.Fatalf("expected model in failure message: %s", msg)
	}
	if !strings.Contains(msg, "畫一隻貓") {
		t.Fatalf("expected prompt in failure message: %s", msg)
	}
	if !strings.Contains(msg, "5 次") {
		t.Fatalf("expected retry limit in failure message: %s", msg)
	}
}
