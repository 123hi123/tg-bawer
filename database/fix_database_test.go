package database

import "testing"

func TestFixDatabaseRepairsDefaultsAndDeletesInvalidServices(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// user 1: create two services then break default flags into multiple defaults
	id1, err := db.AddUserService(1, "standard", "valid-1", "key-valid-1", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService user1 first failed: %v", err)
	}
	id2, err := db.AddUserService(1, "custom", "valid-2", "key-valid-2", "https://example.com", "", "", "", false)
	if err != nil {
		t.Fatalf("AddUserService user1 second failed: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE user_services SET is_default = TRUE WHERE id IN (?, ?)`, id1, id2); err != nil {
		t.Fatalf("break user1 defaults failed: %v", err)
	}

	// user 2: create two services then remove all defaults
	_, err = db.AddUserService(2, "standard", "user2-valid-1", "key-valid-3", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService user2 first failed: %v", err)
	}
	user2NewestID, err := db.AddUserService(2, "custom", "user2-valid-2", "key-valid-4", "https://example.org", "", "", "", false)
	if err != nil {
		t.Fatalf("AddUserService user2 second failed: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE user_services SET is_default = FALSE WHERE user_id = 2`); err != nil {
		t.Fatalf("break user2 defaults failed: %v", err)
	}

	// invalid services: one bad api key, one bad base url
	if _, err := db.AddUserService(3, "standard", "invalid-api", "小男娘", "", "", "", "", true); err != nil {
		t.Fatalf("AddUserService invalid api failed: %v", err)
	}
	if _, err := db.AddUserService(4, "custom", "invalid-url", "key-valid-5", "ftp://bad.example.com", "", "", "", true); err != nil {
		t.Fatalf("AddUserService invalid url failed: %v", err)
	}

	report, err := db.FixDatabase()
	if err != nil {
		t.Fatalf("FixDatabase failed: %v", err)
	}

	if !report.IntegrityOK {
		t.Fatalf("expected integrity check ok, got %+v", report.IntegrityMessages)
	}
	if report.InvalidServicesDeleted != 2 {
		t.Fatalf("expected 2 invalid services deleted, got %d", report.InvalidServicesDeleted)
	}
	if report.UsersDefaultFixed != 2 {
		t.Fatalf("expected 2 users default fixed, got %d", report.UsersDefaultFixed)
	}

	user1Services, err := db.GetUserServices(1)
	if err != nil {
		t.Fatalf("GetUserServices user1 failed: %v", err)
	}
	user1DefaultCount := 0
	for _, service := range user1Services {
		if service.IsDefault {
			user1DefaultCount++
		}
	}
	if user1DefaultCount != 1 {
		t.Fatalf("expected user1 to have exactly 1 default, got %d", user1DefaultCount)
	}

	user2Default, err := db.GetDefaultUserService(2)
	if err != nil {
		t.Fatalf("GetDefaultUserService user2 failed: %v", err)
	}
	if user2Default == nil || user2Default.ID != user2NewestID {
		t.Fatalf("expected user2 default=%d, got %+v", user2NewestID, user2Default)
	}

	user3Services, err := db.GetUserServices(3)
	if err != nil {
		t.Fatalf("GetUserServices user3 failed: %v", err)
	}
	if len(user3Services) != 0 {
		t.Fatalf("expected invalid api service deleted, got %+v", user3Services)
	}

	user4Services, err := db.GetUserServices(4)
	if err != nil {
		t.Fatalf("GetUserServices user4 failed: %v", err)
	}
	if len(user4Services) != 0 {
		t.Fatalf("expected invalid url service deleted, got %+v", user4Services)
	}
}

func TestFixDatabaseLeavesHealthyDataUntouched(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	serviceID, err := db.AddUserService(9, "standard", "healthy", "healthy-key", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService failed: %v", err)
	}

	report, err := db.FixDatabase()
	if err != nil {
		t.Fatalf("FixDatabase failed: %v", err)
	}

	if report.InvalidServicesDeleted != 0 {
		t.Fatalf("expected no invalid services deleted, got %d", report.InvalidServicesDeleted)
	}
	if report.UsersDefaultFixed != 0 {
		t.Fatalf("expected no default repairs, got %d", report.UsersDefaultFixed)
	}

	defaultService, err := db.GetDefaultUserService(9)
	if err != nil {
		t.Fatalf("GetDefaultUserService failed: %v", err)
	}
	if defaultService == nil || defaultService.ID != serviceID {
		t.Fatalf("expected default service to remain %d, got %+v", serviceID, defaultService)
	}
}
