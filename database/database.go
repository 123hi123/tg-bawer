package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Database struct {
	db *sql.DB
}

type SavedPrompt struct {
	ID        int64
	UserID    int64
	Name      string
	Prompt    string
	IsDefault bool
	CreatedAt time.Time
}

type HistoryPrompt struct {
	ID     int64
	UserID int64
	Prompt string
	UsedAt time.Time
}

type UserService struct {
	ID        int64
	UserID    int64
	Name      string
	Type      string
	APIKey    string
	BaseURL   string
	ProjectID string
	Location  string
	Model     string
	IsDefault bool
	CreatedAt time.Time
}

type FailedGeneration struct {
	ID               int64
	UserID           int64
	ChatID           int64
	ReplyToMessageID int64
	Payload          string
	LastError        string
	RetryCount       int
	CreatedAt        time.Time
	LastRetryAt      *time.Time
}

func NewDatabase(dataDir string) (*Database, error) {
	// 確保資料夾存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, "bot.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	d := &Database{db: db}
	if err := d.init(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Database) init() error {
	// 建立保存的 Prompt 表
	_, err := d.db.Exec(`
		CREATE TABLE IF NOT EXISTS saved_prompts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			prompt TEXT NOT NULL,
			is_default BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, name)
		)
	`)
	if err != nil {
		return err
	}

	// 建立使用歷史表
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS prompt_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			prompt TEXT NOT NULL,
			used_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 建立使用者設定表
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_settings (
			user_id INTEGER PRIMARY KEY,
			default_quality TEXT DEFAULT '2K',
			default_prompt_id INTEGER,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	// 建立使用者服務設定表
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_services (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			service_type TEXT NOT NULL,
			api_key TEXT NOT NULL,
			base_url TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			location TEXT DEFAULT '',
			model TEXT DEFAULT '',
			is_default BOOLEAN DEFAULT FALSE,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, name)
		)
	`)
	if err != nil {
		return err
	}

	// 建立生成失敗重試佇列表
	_, err = d.db.Exec(`
		CREATE TABLE IF NOT EXISTS failed_generations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			chat_id INTEGER NOT NULL,
			reply_to_message_id INTEGER DEFAULT 0,
			payload TEXT NOT NULL,
			last_error TEXT,
			retry_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_retry_at DATETIME
		)
	`)
	return err
}

// SavePrompt 保存指定的 Prompt
func (d *Database) SavePrompt(userID int64, name, prompt string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO saved_prompts (user_id, name, prompt, created_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, userID, name, prompt)
	return err
}

// GetSavedPrompts 取得使用者保存的所有 Prompt
func (d *Database) GetSavedPrompts(userID int64) ([]SavedPrompt, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, name, prompt, is_default, created_at
		FROM saved_prompts
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prompts []SavedPrompt
	for rows.Next() {
		var p SavedPrompt
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Prompt, &p.IsDefault, &p.CreatedAt); err != nil {
			return nil, err
		}
		prompts = append(prompts, p)
	}
	return prompts, nil
}

// SetDefaultPrompt 設定預設 Prompt
func (d *Database) SetDefaultPrompt(userID int64, promptID int64) error {
	// 先清除其他預設
	_, err := d.db.Exec(`UPDATE saved_prompts SET is_default = FALSE WHERE user_id = ?`, userID)
	if err != nil {
		return err
	}
	// 設定新預設
	_, err = d.db.Exec(`UPDATE saved_prompts SET is_default = TRUE WHERE id = ? AND user_id = ?`, promptID, userID)
	return err
}

// GetDefaultPrompt 取得使用者的預設 Prompt
func (d *Database) GetDefaultPrompt(userID int64) (*SavedPrompt, error) {
	row := d.db.QueryRow(`
		SELECT id, user_id, name, prompt, is_default, created_at
		FROM saved_prompts
		WHERE user_id = ? AND is_default = TRUE
	`, userID)

	var p SavedPrompt
	if err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Prompt, &p.IsDefault, &p.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// AddToHistory 新增到使用歷史
func (d *Database) AddToHistory(userID int64, prompt string) error {
	_, err := d.db.Exec(`
		INSERT INTO prompt_history (user_id, prompt)
		VALUES (?, ?)
	`, userID, prompt)
	return err
}

// GetHistory 取得使用歷史
func (d *Database) GetHistory(userID int64, limit int) ([]HistoryPrompt, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, prompt, used_at
		FROM prompt_history
		WHERE user_id = ?
		ORDER BY used_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []HistoryPrompt
	for rows.Next() {
		var h HistoryPrompt
		if err := rows.Scan(&h.ID, &h.UserID, &h.Prompt, &h.UsedAt); err != nil {
			return nil, err
		}
		history = append(history, h)
	}
	return history, nil
}

// GetUserSettings 取得使用者設定
func (d *Database) GetUserSettings(userID int64) (string, error) {
	row := d.db.QueryRow(`SELECT default_quality FROM user_settings WHERE user_id = ?`, userID)
	var quality string
	if err := row.Scan(&quality); err != nil {
		if err == sql.ErrNoRows {
			return "2K", nil
		}
		return "", err
	}
	return quality, nil
}

// SetUserSettings 設定使用者預設畫質
func (d *Database) SetUserSettings(userID int64, quality string) error {
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO user_settings (user_id, default_quality, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, userID, quality)
	return err
}

// DeletePrompt 刪除保存的 Prompt
func (d *Database) DeletePrompt(userID int64, promptID int64) error {
	_, err := d.db.Exec(`DELETE FROM saved_prompts WHERE id = ? AND user_id = ?`, promptID, userID)
	return err
}

func (d *Database) AddUserService(userID int64, serviceType, name, apiKey, baseURL, projectID, location, model string, setAsDefault bool) (int64, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if setAsDefault {
		if _, err := tx.Exec(`UPDATE user_services SET is_default = FALSE WHERE user_id = ?`, userID); err != nil {
			return 0, err
		}
	}

	result, err := tx.Exec(`
		INSERT INTO user_services (
			user_id, name, service_type, api_key, base_url, project_id, location, model, is_default, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, userID, name, serviceType, apiKey, baseURL, projectID, location, model, setAsDefault)
	if err != nil {
		return 0, err
	}

	serviceID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if !setAsDefault {
		var total int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM user_services WHERE user_id = ?`, userID).Scan(&total); err != nil {
			return 0, err
		}
		if total == 1 {
			if _, err := tx.Exec(`UPDATE user_services SET is_default = TRUE WHERE id = ?`, serviceID); err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return serviceID, nil
}

func (d *Database) GetUserServices(userID int64) ([]UserService, error) {
	rows, err := d.db.Query(`
		SELECT id, user_id, name, service_type, api_key, base_url, project_id, location, model, is_default, created_at
		FROM user_services
		WHERE user_id = ?
		ORDER BY is_default DESC, created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []UserService
	for rows.Next() {
		var service UserService
		if err := rows.Scan(
			&service.ID,
			&service.UserID,
			&service.Name,
			&service.Type,
			&service.APIKey,
			&service.BaseURL,
			&service.ProjectID,
			&service.Location,
			&service.Model,
			&service.IsDefault,
			&service.CreatedAt,
		); err != nil {
			return nil, err
		}
		services = append(services, service)
	}

	return services, nil
}

func (d *Database) GetDefaultUserService(userID int64) (*UserService, error) {
	row := d.db.QueryRow(`
		SELECT id, user_id, name, service_type, api_key, base_url, project_id, location, model, is_default, created_at
		FROM user_services
		WHERE user_id = ? AND is_default = TRUE
		ORDER BY created_at DESC
		LIMIT 1
	`, userID)

	var service UserService
	if err := row.Scan(
		&service.ID,
		&service.UserID,
		&service.Name,
		&service.Type,
		&service.APIKey,
		&service.BaseURL,
		&service.ProjectID,
		&service.Location,
		&service.Model,
		&service.IsDefault,
		&service.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &service, nil
}

func (d *Database) SetDefaultUserService(userID int64, serviceID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE user_services SET is_default = FALSE WHERE user_id = ?`, userID); err != nil {
		return err
	}

	result, err := tx.Exec(`
		UPDATE user_services
		SET is_default = TRUE
		WHERE user_id = ? AND id = ?
	`, userID, serviceID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	return tx.Commit()
}

func (d *Database) DeleteUserService(userID int64, serviceID int64) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var wasDefault bool
	if err := tx.QueryRow(`
		SELECT is_default
		FROM user_services
		WHERE user_id = ? AND id = ?
	`, userID, serviceID).Scan(&wasDefault); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	if _, err := tx.Exec(`
		DELETE FROM user_services
		WHERE user_id = ? AND id = ?
	`, userID, serviceID); err != nil {
		return err
	}

	if wasDefault {
		var nextID int64
		err := tx.QueryRow(`
			SELECT id
			FROM user_services
			WHERE user_id = ?
			ORDER BY created_at DESC
			LIMIT 1
		`, userID).Scan(&nextID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil {
			if _, err := tx.Exec(`UPDATE user_services SET is_default = TRUE WHERE id = ?`, nextID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (d *Database) AddFailedGeneration(userID, chatID, replyToMessageID int64, payload, lastError string) error {
	_, err := d.db.Exec(`
		INSERT INTO failed_generations (
			user_id, chat_id, reply_to_message_id, payload, last_error, retry_count, created_at
		) VALUES (?, ?, ?, ?, ?, 0, CURRENT_TIMESTAMP)
	`, userID, chatID, replyToMessageID, payload, lastError)
	return err
}

func (d *Database) GetRandomFailedGeneration() (*FailedGeneration, error) {
	row := d.db.QueryRow(`
		SELECT id, user_id, chat_id, reply_to_message_id, payload, last_error, retry_count, created_at, last_retry_at
		FROM failed_generations
		ORDER BY RANDOM()
		LIMIT 1
	`)

	var failed FailedGeneration
	var lastRetry sql.NullTime
	if err := row.Scan(
		&failed.ID,
		&failed.UserID,
		&failed.ChatID,
		&failed.ReplyToMessageID,
		&failed.Payload,
		&failed.LastError,
		&failed.RetryCount,
		&failed.CreatedAt,
		&lastRetry,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if lastRetry.Valid {
		failed.LastRetryAt = &lastRetry.Time
	}

	return &failed, nil
}

func (d *Database) MarkFailedGenerationRetry(id int64, lastError string) error {
	_, err := d.db.Exec(`
		UPDATE failed_generations
		SET retry_count = retry_count + 1,
		    last_error = ?,
		    last_retry_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, lastError, id)
	return err
}

func (d *Database) DeleteFailedGeneration(id int64) error {
	_, err := d.db.Exec(`DELETE FROM failed_generations WHERE id = ?`, id)
	return err
}

func (d *Database) Close() error {
	return d.db.Close()
}
