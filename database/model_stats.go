package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ModelRequestStat struct {
	OwnerUserID  int64
	Provider     string
	ModelName    string
	TotalCount   int64
	SuccessCount int64
	FailureCount int64
	UpdatedAt    time.Time
}

func normalizeModelStatValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func (d *Database) ensureModelRequestStatsTable() error {
	rows, err := d.db.Query(`PRAGMA table_info(model_request_stats)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	columnCount := 0
	hasOwnerUserID := false
	for rows.Next() {
		columnCount++
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return err
		}
		if name == "owner_user_id" {
			hasOwnerUserID = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if columnCount == 0 {
		_, err := d.db.Exec(`
			CREATE TABLE IF NOT EXISTS model_request_stats (
				owner_user_id INTEGER NOT NULL DEFAULT 0,
				provider TEXT NOT NULL,
				model_name TEXT NOT NULL,
				total_count INTEGER NOT NULL DEFAULT 0,
				success_count INTEGER NOT NULL DEFAULT 0,
				failure_count INTEGER NOT NULL DEFAULT 0,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				PRIMARY KEY (owner_user_id, provider, model_name)
			)
		`)
		return err
	}

	if hasOwnerUserID {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE model_request_stats_new (
			owner_user_id INTEGER NOT NULL DEFAULT 0,
			provider TEXT NOT NULL,
			model_name TEXT NOT NULL,
			total_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			failure_count INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (owner_user_id, provider, model_name)
		)
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO model_request_stats_new (
			owner_user_id, provider, model_name, total_count, success_count, failure_count, updated_at
		)
		SELECT 0, provider, model_name, total_count, success_count, failure_count, updated_at
		FROM model_request_stats
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`DROP TABLE model_request_stats`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE model_request_stats_new RENAME TO model_request_stats`); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *Database) RecordModelRequest(ownerUserID int64, provider, model string, totalDelta, successDelta, failureDelta int64) error {
	provider = normalizeModelStatValue(provider)
	model = normalizeModelStatValue(model)

	_, err := d.db.Exec(`
		INSERT INTO model_request_stats (
			owner_user_id,
			provider,
			model_name,
			total_count,
			success_count,
			failure_count,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(owner_user_id, provider, model_name) DO UPDATE SET
			total_count = model_request_stats.total_count + excluded.total_count,
			success_count = model_request_stats.success_count + excluded.success_count,
			failure_count = model_request_stats.failure_count + excluded.failure_count,
			updated_at = CURRENT_TIMESTAMP
	`, ownerUserID, provider, model, totalDelta, successDelta, failureDelta)
	return err
}

func (d *Database) GetModelRequestStats() ([]ModelRequestStat, error) {
	return d.GetGlobalModelRequestStats()
}

func (d *Database) GetUserModelRequestStats(ownerUserID int64) ([]ModelRequestStat, error) {
	rows, err := d.db.Query(`
		SELECT owner_user_id, provider, model_name, total_count, success_count, failure_count, updated_at
		FROM model_request_stats
		WHERE owner_user_id = ?
		ORDER BY failure_count DESC, total_count DESC, provider ASC, model_name ASC
	`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ModelRequestStat
	for rows.Next() {
		var stat ModelRequestStat
		if err := rows.Scan(
			&stat.OwnerUserID,
			&stat.Provider,
			&stat.ModelName,
			&stat.TotalCount,
			&stat.SuccessCount,
			&stat.FailureCount,
			&stat.UpdatedAt,
		); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

func (d *Database) GetGlobalModelRequestStats() ([]ModelRequestStat, error) {
	rows, err := d.db.Query(`
		SELECT
			0 AS owner_user_id,
			provider,
			model_name,
			SUM(total_count) AS total_count,
			SUM(success_count) AS success_count,
			SUM(failure_count) AS failure_count,
			MAX(updated_at) AS updated_at
		FROM model_request_stats
		GROUP BY provider, model_name
		ORDER BY failure_count DESC, total_count DESC, provider ASC, model_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []ModelRequestStat
	for rows.Next() {
		var (
			stat         ModelRequestStat
			rawUpdatedAt string
		)
		if err := rows.Scan(
			&stat.OwnerUserID,
			&stat.Provider,
			&stat.ModelName,
			&stat.TotalCount,
			&stat.SuccessCount,
			&stat.FailureCount,
			&rawUpdatedAt,
		); err != nil {
			return nil, err
		}
		if rawUpdatedAt != "" {
			parsed, parseErr := parseSQLiteTime(rawUpdatedAt)
			if parseErr != nil {
				return nil, parseErr
			}
			stat.UpdatedAt = parsed
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

func parseSQLiteTime(raw string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported sqlite time format: %s", raw)
}
