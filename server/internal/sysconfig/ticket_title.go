package sysconfig

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TicketTitleKey is the single editable source of the printed ticket title.
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
