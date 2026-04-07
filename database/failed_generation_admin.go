package database

import (
	"database/sql"
	"fmt"
	"strings"
)

func (d *Database) ListFailedGenerationsBySources(sources []string) ([]FailedGeneration, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(sources))
	args := make([]any, len(sources))
	for i, source := range sources {
		placeholders[i] = "?"
		args[i] = source
	}

	rows, err := d.db.Query(fmt.Sprintf(`
		SELECT id, user_id, chat_id, reply_to_message_id, payload, COALESCE(source, 'google'), last_error, retry_count, created_at, last_retry_at
		FROM failed_generations
		WHERE COALESCE(source, 'google') IN (%s)
		ORDER BY id ASC
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []FailedGeneration
	for rows.Next() {
		var item FailedGeneration
		var lastRetry sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.ChatID,
			&item.ReplyToMessageID,
			&item.Payload,
			&item.Source,
			&item.LastError,
			&item.RetryCount,
			&item.CreatedAt,
			&lastRetry,
		); err != nil {
			return nil, err
		}
		if lastRetry.Valid {
			item.LastRetryAt = &lastRetry.Time
		}
		items = append(items, item)
	}

	return items, rows.Err()
}
