package database

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type DBRuntimeStats struct {
	OpenConnections int
	InUse           int
	Idle            int
	WaitCount       int64
	WaitDuration    time.Duration
}

func (d *Database) GetDBRuntimeStats() DBRuntimeStats {
	stats := d.db.Stats()
	return DBRuntimeStats{
		OpenConnections: stats.OpenConnections,
		InUse:           stats.InUse,
		Idle:            stats.Idle,
		WaitCount:       stats.WaitCount,
		WaitDuration:    stats.WaitDuration,
	}
}

func (d *Database) SetSystemInt(key string, value int) error {
	_, err := d.db.Exec(`
		INSERT INTO system_settings (setting_key, setting_value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(setting_key) DO UPDATE SET
			setting_value = excluded.setting_value,
			updated_at = excluded.updated_at
	`, key, strconv.Itoa(value))
	return err
}

func (d *Database) GetSystemInt(key string, defaultValue int) (int, error) {
	var raw string
	err := d.db.QueryRow(`
		SELECT setting_value
		FROM system_settings
		WHERE setting_key = ?
	`, key).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return defaultValue, nil
		}
		return defaultValue, err
	}

	n, parseErr := strconv.Atoi(strings.TrimSpace(raw))
	if parseErr != nil {
		return defaultValue, fmt.Errorf("invalid int setting %s=%q: %w", key, raw, parseErr)
	}
	return n, nil
}
