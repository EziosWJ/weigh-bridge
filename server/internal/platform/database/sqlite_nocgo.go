//go:build !cgo

package database

import (
	"context"
	"errors"
)

// PostgreSQL works without CGO; the existing SQLite driver requires it.
func NormalizeError(err error) error { return err }

func BackupSQLite(context.Context, string, string) error {
	return errors.New("SQLite 备份需要 CGO_ENABLED=1 和目标平台 C 编译器")
}

func VerifySQLiteFile(context.Context, string) error {
	return errors.New("SQLite 验证需要 CGO_ENABLED=1 和目标平台 C 编译器")
}
