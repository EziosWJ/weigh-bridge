//go:build cgo

package database

import (
	"errors"
	platformerrors "github.com/EziosWJ/weighbridge/server/internal/platform/errors"
	"github.com/mattn/go-sqlite3"
)

// NormalizeError maps SQLite busy/locked errors without exposing driver types.
func NormalizeError(err error) error {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && (sqliteErr.Code == sqlite3.ErrBusy || sqliteErr.Code == sqlite3.ErrLocked) {
		return platformerrors.ErrTemporarilyUnavailable
	}
	return err
}
