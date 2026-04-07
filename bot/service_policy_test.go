package bot

import (
	"testing"

	"tg-bawer/database"
	"tg-bawer/gemini"
	"tg-bawer/grok"
)

const testAdminUserID int64 = 99999

func TestShouldApplyPublicServiceRateLimit(t *testing.T) {
	db, err := database.NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	b := &Bot{
		db:          db,
		adminUserID: testAdminUserID,
	}

	if b.shouldApplyPublicServiceRateLimit(testAdminUserID) {
		t.Fatalf("admin should bypass rate limit")
	}

	if !b.shouldApplyPublicServiceRateLimit(1001) {
		t.Fatalf("user without private service should be rate limited")
	}

	privateServiceID, err := db.AddUserService(1002, gemini.ServiceTypeStandard, "private", "key", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService private failed: %v", err)
	}
	if privateServiceID == 0 {
		t.Fatalf("expected private service id")
	}
	if b.shouldApplyPublicServiceRateLimit(1002) {
		t.Fatalf("user with private service should bypass rate limit")
	}

	publicOnlyUserID := int64(1003)
	publicServiceID, err := db.AddUserService(publicOnlyUserID, gemini.ServiceTypeStandard, "public", "key", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService public failed: %v", err)
	}
	if err := db.SetUserServicePublic(publicOnlyUserID, publicServiceID, true); err != nil {
		t.Fatalf("SetUserServicePublic failed: %v", err)
	}
	if !b.shouldApplyPublicServiceRateLimit(publicOnlyUserID) {
		t.Fatalf("user with only public service should still be rate limited")
	}
}

func TestCanAddPrivateServiceQuota(t *testing.T) {
	db, err := database.NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	b := &Bot{db: db}
	userID := int64(2001)

	if ok, err := b.canAddPrivateService(userID, gemini.ServiceTypeStandard); err != nil || !ok {
		t.Fatalf("expected first google service allowed, ok=%v err=%v", ok, err)
	}

	if _, err := db.AddUserService(userID, gemini.ServiceTypeStandard, "g1", "key", "", "", "", "", true); err != nil {
		t.Fatalf("AddUserService google failed: %v", err)
	}

	if ok, err := b.canAddPrivateService(userID, gemini.ServiceTypeCustom); err != nil || ok {
		t.Fatalf("expected second google-family private service denied, ok=%v err=%v", ok, err)
	}

	if ok, err := b.canAddPrivateService(userID, grok.ServiceTypeGrok); err != nil || !ok {
		t.Fatalf("expected first grok service allowed, ok=%v err=%v", ok, err)
	}

	if _, err := db.AddUserService(userID, grok.ServiceTypeGrok, "gr1", "key", "http://example.com", "", "", "", false); err != nil {
		t.Fatalf("AddUserService grok failed: %v", err)
	}

	if ok, err := b.canAddPrivateService(userID, grok.ServiceTypeGrok); err != nil || ok {
		t.Fatalf("expected second grok private service denied, ok=%v err=%v", ok, err)
	}
}

func TestCanAddPrivateServiceQuota_AdminBypass(t *testing.T) {
	db, err := database.NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	b := &Bot{db: db, adminUserID: testAdminUserID}

	if _, err := db.AddUserService(testAdminUserID, gemini.ServiceTypeStandard, "g1", "key", "", "", "", "", true); err != nil {
		t.Fatalf("AddUserService admin google failed: %v", err)
	}
	if _, err := db.AddUserService(testAdminUserID, grok.ServiceTypeGrok, "gr1", "key", "http://example.com", "", "", "", false); err != nil {
		t.Fatalf("AddUserService admin grok failed: %v", err)
	}

	if ok, err := b.canAddPrivateService(testAdminUserID, gemini.ServiceTypeVertex); err != nil || !ok {
		t.Fatalf("expected admin google quota bypass, ok=%v err=%v", ok, err)
	}
	if ok, err := b.canAddPrivateService(testAdminUserID, grok.ServiceTypeGrok); err != nil || !ok {
		t.Fatalf("expected admin grok quota bypass, ok=%v err=%v", ok, err)
	}
}
