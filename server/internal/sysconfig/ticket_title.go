package sysconfig

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TicketTitleKey is the single editable source of the printed ticket title.
// The previous YAML setting (weighing.ticket_title) is only read once during
// upgrade by InstallLegacyTicketTitle.
const TicketTitleKey = "weighing.ticket-title"

// DefaultTicketTitle keeps the paper layout stable when the configuration row
// is absent, disabled or invalid.
const DefaultTicketTitle = "酒厂过磅单"

const maxTicketTitleRunes = 100

// ReadTicketTitle returns the effective title for a new or legacy ticket. When
// lock is true the shared row lock is held until the surrounding business
// transaction commits, so the caller can freeze the same title onto a ticket.
func ReadTicketTitle(ctx context.Context, db *gorm.DB, lock bool) (string, error) {
	var row Config
	query := db.WithContext(ctx).Where("config_key=? AND deleted=0", TicketTitleKey)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "SHARE"})
	}
	err := query.Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultTicketTitle, nil
	}
	if err != nil {
		return "", err
	}
	if row.Status != enabled || row.ValueType != "TEXT" {
		return DefaultTicketTitle, nil
	}
	title, ok := normalizeTicketTitle(row.ConfigValue)
	if !ok {
		return DefaultTicketTitle, nil
	}
	return title, nil
}

// InstallLegacyTicketTitle performs the one-time upgrade handoff from the
// former YAML setting. The new row is seeded with an empty value; the first
// application start replaces that empty value with the deployment's previous
// title. Once a manager edits the title it is never overwritten again.
// It returns the effective title so callers can use the same value to fix the
// display baseline of tickets that predate title snapshots.
func InstallLegacyTicketTitle(ctx context.Context, db *gorm.DB, legacy string) (string, error) {
	legacyTitle, ok := normalizeTicketTitle(legacy)
	if !ok {
		legacyTitle = DefaultTicketTitle
	}
	var row Config
	err := db.WithContext(ctx).Where("config_key=? AND deleted=0", TicketTitleKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Migration not applied yet (for example an isolated unit test); the
		// read path still falls back to the stable default.
		return DefaultTicketTitle, nil
	}
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(row.ConfigValue) == "" {
		if err := db.WithContext(ctx).Model(&Config{}).Where("id=? AND config_key=?", row.ID, TicketTitleKey).Updates(map[string]any{
			"config_value": legacyTitle,
			"update_time":  time.Now().UTC(),
		}).Error; err != nil {
			return "", err
		}
	}
	return ReadTicketTitle(ctx, db, false)
}

func normalizeTicketTitle(value string) (string, bool) {
	title := strings.TrimSpace(value)
	if title == "" || utf8.RuneCountInString(title) > maxTicketTitleRunes {
		return "", false
	}
	if strings.ContainsAny(title, "\r\n\t") {
		return "", false
	}
	return title, true
}
