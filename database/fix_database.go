package database

import (
	"database/sql"
	"net/url"
	"strings"
	"unicode"
)

type DatabaseFixReport struct {
	IntegrityOK            bool
	IntegrityMessages      []string
	UsersDefaultFixed      int
	InvalidServicesDeleted int
	InvalidServiceIDs      []int64
}

func (d *Database) FixDatabase() (*DatabaseFixReport, error) {
	report := &DatabaseFixReport{}

	integrityMessages, err := d.runIntegrityCheck()
	if err != nil {
		return nil, err
	}
	report.IntegrityMessages = integrityMessages
	report.IntegrityOK = len(integrityMessages) == 1 && strings.EqualFold(integrityMessages[0], "ok")

	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	invalidServiceIDs, err := collectInvalidServiceIDs(tx)
	if err != nil {
		return nil, err
	}
	for _, serviceID := range invalidServiceIDs {
		if _, err := tx.Exec(`DELETE FROM user_services WHERE id = ?`, serviceID); err != nil {
			return nil, err
		}
	}
	report.InvalidServiceIDs = invalidServiceIDs
	report.InvalidServicesDeleted = len(invalidServiceIDs)

	fixedUsers, err := repairUserServiceDefaults(tx)
	if err != nil {
		return nil, err
	}
	report.UsersDefaultFixed = fixedUsers

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return report, nil
}

func (d *Database) runIntegrityCheck() ([]string, error) {
	rows, err := d.db.Query(`PRAGMA integrity_check`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []string
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func collectInvalidServiceIDs(tx interface {
	Query(query string, args ...any) (*sql.Rows, error)
}) ([]int64, error) {
	rows, err := tx.Query(`
		SELECT id, api_key, base_url
		FROM user_services
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var (
			id      int64
			apiKey  string
			baseURL string
		)
		if err := rows.Scan(&id, &apiKey, &baseURL); err != nil {
			return nil, err
		}
		if isClearlyInvalidAPIKey(apiKey) || isClearlyInvalidBaseURL(baseURL) {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

func repairUserServiceDefaults(tx *sql.Tx) (int, error) {
	rows, err := tx.Query(`SELECT DISTINCT user_id FROM user_services ORDER BY user_id ASC`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var userIDs []int64
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			return 0, err
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	fixedUsers := 0
	for _, userID := range userIDs {
		var (
			keeperID     int64
			defaultCount int
		)

		err := tx.QueryRow(`
			SELECT id
			FROM user_services
			WHERE user_id = ?
			ORDER BY is_default DESC, created_at DESC, id DESC
			LIMIT 1
		`, userID).Scan(&keeperID)
		if err != nil {
			return 0, err
		}

		if err := tx.QueryRow(`
			SELECT COUNT(*)
			FROM user_services
			WHERE user_id = ? AND is_default = TRUE
		`, userID).Scan(&defaultCount); err != nil {
			return 0, err
		}

		if defaultCount == 1 {
			var currentDefaultID int64
			if err := tx.QueryRow(`
				SELECT id
				FROM user_services
				WHERE user_id = ? AND is_default = TRUE
				ORDER BY created_at DESC, id DESC
				LIMIT 1
			`, userID).Scan(&currentDefaultID); err != nil {
				return 0, err
			}
			if currentDefaultID == keeperID {
				continue
			}
		}

		if _, err := tx.Exec(`
			UPDATE user_services
			SET is_default = CASE WHEN id = ? THEN TRUE ELSE FALSE END
			WHERE user_id = ?
		`, keeperID, userID); err != nil {
			return 0, err
		}
		fixedUsers++
	}

	return fixedUsers, nil
}

func isClearlyInvalidAPIKey(apiKey string) bool {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return true
	}
	for _, r := range apiKey {
		if unicode.IsSpace(r) || unicode.In(r, unicode.Han) {
			return true
		}
	}
	return false
}

func isClearlyInvalidBaseURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return true
	}
	if parsed.Host == "" {
		return true
	}
	return parsed.Scheme != "http" && parsed.Scheme != "https"
}
