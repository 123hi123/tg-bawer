package database

import "testing"

func TestUserServiceCRUD(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	firstID, err := db.AddUserService(1, "standard", "main", "key1", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService first failed: %v", err)
	}
	secondID, err := db.AddUserService(1, "custom", "proxy", "key2", "https://example.com", "", "", "", false)
	if err != nil {
		t.Fatalf("AddUserService second failed: %v", err)
	}

	defaultService, err := db.GetDefaultUserService(1)
	if err != nil {
		t.Fatalf("GetDefaultUserService failed: %v", err)
	}
	if defaultService == nil || defaultService.ID != firstID {
		t.Fatalf("expected default id %d, got %+v", firstID, defaultService)
	}

	if err := db.SetDefaultUserService(1, secondID); err != nil {
		t.Fatalf("SetDefaultUserService failed: %v", err)
	}
	defaultService, err = db.GetDefaultUserService(1)
	if err != nil {
		t.Fatalf("GetDefaultUserService after switch failed: %v", err)
	}
	if defaultService == nil || defaultService.ID != secondID {
		t.Fatalf("expected default id %d after switch, got %+v", secondID, defaultService)
	}

	if err := db.DeleteUserService(1, secondID); err != nil {
		t.Fatalf("DeleteUserService failed: %v", err)
	}
	defaultService, err = db.GetDefaultUserService(1)
	if err != nil {
		t.Fatalf("GetDefaultUserService after delete failed: %v", err)
	}
	if defaultService == nil || defaultService.ID != firstID {
		t.Fatalf("expected fallback default id %d, got %+v", firstID, defaultService)
	}
}

func TestSetUserServicePublic(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	id, err := db.AddUserService(1, "standard", "main", "key1", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService failed: %v", err)
	}

	// New service should default to private
	services, err := db.GetUserServices(1)
	if err != nil {
		t.Fatalf("GetUserServices failed: %v", err)
	}
	if len(services) != 1 || services[0].IsPublic {
		t.Fatalf("expected service to default to private, got %+v", services)
	}

	// Set to public
	if err := db.SetUserServicePublic(1, id, true); err != nil {
		t.Fatalf("SetUserServicePublic(true) failed: %v", err)
	}
	services, err = db.GetUserServices(1)
	if err != nil {
		t.Fatalf("GetUserServices after pub failed: %v", err)
	}
	if !services[0].IsPublic {
		t.Fatalf("expected service to be public, got %+v", services[0])
	}

	// Set back to private
	if err := db.SetUserServicePublic(1, id, false); err != nil {
		t.Fatalf("SetUserServicePublic(false) failed: %v", err)
	}
	services, err = db.GetUserServices(1)
	if err != nil {
		t.Fatalf("GetUserServices after pri failed: %v", err)
	}
	if services[0].IsPublic {
		t.Fatalf("expected service to be private, got %+v", services[0])
	}

	// Non-existent service should return ErrNoRows
	err = db.SetUserServicePublic(1, 9999, true)
	if err == nil {
		t.Fatalf("expected error for non-existent service")
	}
}

func TestFailedGenerationQueue(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	if err := db.AddFailedGeneration(10, 20, 30, `{"prompt":"x"}`, "boom", "google"); err != nil {
		t.Fatalf("AddFailedGeneration failed: %v", err)
	}

	task, err := db.GetRandomFailedGeneration()
	if err != nil {
		t.Fatalf("GetRandomFailedGeneration failed: %v", err)
	}
	if task == nil {
		t.Fatalf("expected one failed generation task")
	}
	if task.UserID != 10 || task.ChatID != 20 || task.ReplyToMessageID != 30 {
		t.Fatalf("unexpected task: %+v", task)
	}

	if err := db.MarkFailedGenerationRetry(task.ID, "still boom"); err != nil {
		t.Fatalf("MarkFailedGenerationRetry failed: %v", err)
	}

	task, err = db.GetRandomFailedGeneration()
	if err != nil {
		t.Fatalf("GetRandomFailedGeneration second read failed: %v", err)
	}
	if task == nil || task.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %+v", task)
	}
	if task.Source != "google" {
		t.Fatalf("expected source=google, got %s", task.Source)
	}

	if err := db.DeleteFailedGeneration(task.ID); err != nil {
		t.Fatalf("DeleteFailedGeneration failed: %v", err)
	}
	task, err = db.GetRandomFailedGeneration()
	if err != nil {
		t.Fatalf("GetRandomFailedGeneration after delete failed: %v", err)
	}
	if task != nil {
		t.Fatalf("expected empty queue, got %+v", task)
	}
}

func TestGetRandomFailedGenerationReturnsQueueHead(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	if err := db.AddFailedGeneration(1, 10, 0, `{"prompt":"first"}`, "", "google"); err != nil {
		t.Fatalf("AddFailedGeneration first failed: %v", err)
	}
	if err := db.AddFailedGeneration(2, 10, 0, `{"prompt":"second"}`, "", "grok"); err != nil {
		t.Fatalf("AddFailedGeneration second failed: %v", err)
	}

	task, err := db.GetRandomFailedGeneration()
	if err != nil {
		t.Fatalf("GetRandomFailedGeneration failed: %v", err)
	}
	if task == nil {
		t.Fatalf("expected queue head task")
	}
	if task.UserID != 1 {
		t.Fatalf("expected first queued task (user 1), got %+v", task)
	}
}

func TestUpdateFailedGenerationRetryErrorDoesNotIncrementCounter(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	if err := db.AddFailedGeneration(10, 20, 30, `{"prompt":"x"}`, "boom", "google"); err != nil {
		t.Fatalf("AddFailedGeneration failed: %v", err)
	}

	task, err := db.GetRandomFailedGeneration()
	if err != nil {
		t.Fatalf("GetRandomFailedGeneration failed: %v", err)
	}
	if task == nil {
		t.Fatalf("expected task")
	}

	if err := db.UpdateFailedGenerationRetryError(task.ID, "rate limited"); err != nil {
		t.Fatalf("UpdateFailedGenerationRetryError failed: %v", err)
	}

	task, err = db.GetRandomFailedGeneration()
	if err != nil {
		t.Fatalf("GetRandomFailedGeneration second read failed: %v", err)
	}
	if task == nil {
		t.Fatalf("expected task after update")
	}
	if task.RetryCount != 0 {
		t.Fatalf("expected retry_count to remain 0, got %d", task.RetryCount)
	}
	if task.LastError != "rate limited" {
		t.Fatalf("expected last error updated, got %q", task.LastError)
	}
	if task.LastRetryAt == nil {
		t.Fatalf("expected last_retry_at to be set")
	}
}

func TestGetFailedGenerationCounts(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// Add tasks with various source types (including legacy values)
	if err := db.AddFailedGeneration(42, 1, 0, `{"prompt":"p"}`, "", "google_image"); err != nil {
		t.Fatalf("AddFailedGeneration google_image failed: %v", err)
	}
	if err := db.AddFailedGeneration(42, 1, 0, `{"prompt":"p"}`, "", "google_image"); err != nil {
		t.Fatalf("AddFailedGeneration google_image 2 failed: %v", err)
	}
	if err := db.AddFailedGeneration(42, 1, 0, `{"prompt":"p"}`, "", "grok_image"); err != nil {
		t.Fatalf("AddFailedGeneration grok_image failed: %v", err)
	}
	if err := db.AddFailedGeneration(42, 1, 0, `{"prompt":"p"}`, "", "grok_video"); err != nil {
		t.Fatalf("AddFailedGeneration grok_video failed: %v", err)
	}
	// Legacy source value
	if err := db.AddFailedGeneration(42, 1, 0, `{"prompt":"p"}`, "", "google"); err != nil {
		t.Fatalf("AddFailedGeneration legacy google failed: %v", err)
	}
	// Different user – should not appear in user 42's counts
	if err := db.AddFailedGeneration(99, 1, 0, `{"prompt":"p"}`, "", "grok_image"); err != nil {
		t.Fatalf("AddFailedGeneration other user failed: %v", err)
	}

	counts, err := db.GetFailedGenerationCounts(42)
	if err != nil {
		t.Fatalf("GetFailedGenerationCounts failed: %v", err)
	}

	if counts["google_image"] != 2 {
		t.Errorf("expected google_image=2, got %d", counts["google_image"])
	}
	if counts["grok_image"] != 1 {
		t.Errorf("expected grok_image=1, got %d", counts["grok_image"])
	}
	if counts["grok_video"] != 1 {
		t.Errorf("expected grok_video=1, got %d", counts["grok_video"])
	}
	if counts["google"] != 1 {
		t.Errorf("expected legacy google=1, got %d", counts["google"])
	}
	// Other user's tasks must not bleed in
	if counts["grok_image"] != 1 {
		t.Errorf("other user tasks should not appear, grok_image=%d", counts["grok_image"])
	}
}

func TestGetAllFailedGenerationCounts(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// Add tasks for different users and source types
	if err := db.AddFailedGeneration(42, 1, 0, `{"prompt":"p"}`, "", "google_image"); err != nil {
		t.Fatalf("AddFailedGeneration failed: %v", err)
	}
	if err := db.AddFailedGeneration(42, 1, 0, `{"prompt":"p"}`, "", "grok_image"); err != nil {
		t.Fatalf("AddFailedGeneration failed: %v", err)
	}
	if err := db.AddFailedGeneration(99, 1, 0, `{"prompt":"p"}`, "", "google_image"); err != nil {
		t.Fatalf("AddFailedGeneration failed: %v", err)
	}
	if err := db.AddFailedGeneration(99, 1, 0, `{"prompt":"p"}`, "", "grok_video"); err != nil {
		t.Fatalf("AddFailedGeneration failed: %v", err)
	}

	counts, err := db.GetAllFailedGenerationCounts()
	if err != nil {
		t.Fatalf("GetAllFailedGenerationCounts failed: %v", err)
	}

	// google_image: user 42 + user 99 = 2
	if counts["google_image"] != 2 {
		t.Errorf("expected google_image=2, got %d", counts["google_image"])
	}
	// grok_image: user 42 = 1
	if counts["grok_image"] != 1 {
		t.Errorf("expected grok_image=1, got %d", counts["grok_image"])
	}
	// grok_video: user 99 = 1
	if counts["grok_video"] != 1 {
		t.Errorf("expected grok_video=1, got %d", counts["grok_video"])
	}
}

func TestGetAllFailedGenerationCounts_Empty(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	counts, err := db.GetAllFailedGenerationCounts()
	if err != nil {
		t.Fatalf("GetAllFailedGenerationCounts failed: %v", err)
	}
	if len(counts) != 0 {
		t.Errorf("expected empty counts, got %v", counts)
	}
}

func TestGetPublicServices(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// Add services for two different users
	id1, err := db.AddUserService(1, "standard", "user1-private", "key1", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService failed: %v", err)
	}
	id2, err := db.AddUserService(1, "custom", "user1-public", "key2", "https://example.com", "", "", "", false)
	if err != nil {
		t.Fatalf("AddUserService failed: %v", err)
	}
	_, err = db.AddUserService(2, "standard", "user2-private", "key3", "", "", "", "", true)
	if err != nil {
		t.Fatalf("AddUserService failed: %v", err)
	}
	id4, err := db.AddUserService(2, "grok", "user2-public-grok", "key4", "http://grok-host:8000", "", "", "", false)
	if err != nil {
		t.Fatalf("AddUserService failed: %v", err)
	}

	// Initially no public services
	public, err := db.GetPublicServices()
	if err != nil {
		t.Fatalf("GetPublicServices failed: %v", err)
	}
	if len(public) != 0 {
		t.Fatalf("expected 0 public services, got %d", len(public))
	}

	// Set user1's second service and user2's fourth service as public
	if err := db.SetUserServicePublic(1, id2, true); err != nil {
		t.Fatalf("SetUserServicePublic failed: %v", err)
	}
	if err := db.SetUserServicePublic(2, id4, true); err != nil {
		t.Fatalf("SetUserServicePublic failed: %v", err)
	}

	public, err = db.GetPublicServices()
	if err != nil {
		t.Fatalf("GetPublicServices failed: %v", err)
	}
	if len(public) != 2 {
		t.Fatalf("expected 2 public services, got %d", len(public))
	}

	// Verify the private services are not in the result
	for _, s := range public {
		if s.ID == id1 {
			t.Errorf("private service id=%d should not appear in public list", id1)
		}
	}

	// Verify all returned services are indeed public
	for _, s := range public {
		if !s.IsPublic {
			t.Errorf("service %d should be public", s.ID)
		}
	}

	// Revoke one public service
	if err := db.SetUserServicePublic(1, id2, false); err != nil {
		t.Fatalf("SetUserServicePublic(false) failed: %v", err)
	}
	public, err = db.GetPublicServices()
	if err != nil {
		t.Fatalf("GetPublicServices after revoke failed: %v", err)
	}
	if len(public) != 1 {
		t.Fatalf("expected 1 public service after revoke, got %d", len(public))
	}
	if public[0].ID != id4 {
		t.Errorf("expected remaining public service id=%d, got %d", id4, public[0].ID)
	}
}

func TestImageQueue(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// Add images to queue
	if err := db.AddImageToQueue(1, 100, "file1", ""); err != nil {
		t.Fatalf("AddImageToQueue failed: %v", err)
	}
	if err := db.AddImageToQueue(1, 100, "file2", ""); err != nil {
		t.Fatalf("AddImageToQueue second failed: %v", err)
	}

	// Duplicate should increment ref_count
	if err := db.AddImageToQueue(1, 100, "file1", ""); err != nil {
		t.Fatalf("AddImageToQueue duplicate failed: %v", err)
	}

	items, err := db.GetUserImageQueue(1, 100)
	if err != nil {
		t.Fatalf("GetUserImageQueue failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// file1 should have ref_count=2
	if items[0].FileID != "file1" || items[0].RefCount != 2 {
		t.Fatalf("expected file1 with ref_count=2, got %+v", items[0])
	}

	// Clear queue (decrements ref_count)
	if err := db.ClearUserImageQueue(1, 100); err != nil {
		t.Fatalf("ClearUserImageQueue failed: %v", err)
	}

	items, err = db.GetUserImageQueue(1, 100)
	if err != nil {
		t.Fatalf("GetUserImageQueue after clear failed: %v", err)
	}
	// file1 should still exist with ref_count=1
	if len(items) != 1 {
		t.Fatalf("expected 1 item after clear, got %d", len(items))
	}
	if items[0].FileID != "file1" || items[0].RefCount != 1 {
		t.Fatalf("expected file1 with ref_count=1, got %+v", items[0])
	}
}

func TestBotReplyPromptCRUD(t *testing.T) {
	db, err := NewDatabase(t.TempDir())
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// Save some reply prompts
	if err := db.SaveBotReplyPrompt(100, 1, "draw a cat", "🌐 Google 圖片", "photo"); err != nil {
		t.Fatalf("SaveBotReplyPrompt failed: %v", err)
	}
	if err := db.SaveBotReplyPrompt(100, 2, "draw a dog", "🤖 Grok 圖片", "photo"); err != nil {
		t.Fatalf("SaveBotReplyPrompt second failed: %v", err)
	}
	if err := db.SaveBotReplyPrompt(200, 3, "video prompt", "🎬 Grok 影片", "video"); err != nil {
		t.Fatalf("SaveBotReplyPrompt other chat failed: %v", err)
	}

	// Get by chat
	items, err := db.GetBotReplyPromptsByChat(100)
	if err != nil {
		t.Fatalf("GetBotReplyPromptsByChat failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items for chat 100, got %d", len(items))
	}
	if items[0].Prompt != "draw a cat" || items[0].Caption != "🌐 Google 圖片" {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].Prompt != "draw a dog" {
		t.Fatalf("unexpected second item: %+v", items[1])
	}

	// Get all
	all, err := db.GetAllBotReplyPrompts()
	if err != nil {
		t.Fatalf("GetAllBotReplyPrompts failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 total items, got %d", len(all))
	}

	// Delete
	if err := db.DeleteBotReplyPrompt(items[0].ID); err != nil {
		t.Fatalf("DeleteBotReplyPrompt failed: %v", err)
	}
	items, err = db.GetBotReplyPromptsByChat(100)
	if err != nil {
		t.Fatalf("GetBotReplyPromptsByChat after delete failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after delete, got %d", len(items))
	}

	// Upsert (same chat_id + message_id should replace)
	if err := db.SaveBotReplyPrompt(100, 2, "updated prompt", "🤖 Grok 圖片", "photo"); err != nil {
		t.Fatalf("SaveBotReplyPrompt upsert failed: %v", err)
	}
	items, err = db.GetBotReplyPromptsByChat(100)
	if err != nil {
		t.Fatalf("GetBotReplyPromptsByChat after upsert failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item after upsert, got %d", len(items))
	}
	if items[0].Prompt != "updated prompt" {
		t.Fatalf("expected updated prompt, got %s", items[0].Prompt)
	}
}
