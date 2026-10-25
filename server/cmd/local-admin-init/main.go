// Command local-admin-init sets the seeded administrator password once.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/config"
	platformdatabase "github.com/EziosWJ/weighbridge/server/internal/platform/database"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	var configDir string
	flag.StringVar(&configDir, "config-dir", config.DefaultDir, "directory containing API configuration")
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), configDir, os.Getenv("WEIGHBRIDGE_ADMIN_PASSWORD")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("本地管理员密码已初始化；本厂身份请在管理员网页中完成首次配置。")
}

func run(ctx context.Context, configDir, password string) error {
	if len(password) < 12 || len(password) > 72 || password != strings.TrimSpace(password) {
		return errors.New("WEIGHBRIDGE_ADMIN_PASSWORD 必须为未补空格的 12..72 字节密码")
	}
	cfg, err := config.LoadFromDir(configDir)
	if err != nil {
		return errors.New("无法读取本地 API 配置")
	}
	db, err := platformdatabase.Open(ctx, cfg.Database)
	if err != nil {
		return errors.New("无法连接本地 PostgreSQL")
	}
	defer db.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("无法生成管理员密码摘要")
	}
	return db.GORM.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var admin struct {
			ID int64
		}
		if err := tx.Table("sys_user").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id=1 AND username='admin' AND deleted=0 AND is_builtin=1").Take(&admin).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("seed admin account is missing")
			}
			return errors.New("unable to lock seed admin account")
		}
		var marker int64
		if err := tx.Table("wb_local_admin_bootstrap").Where("id=1").Count(&marker).Error; err != nil {
			return errors.New("local administrator bootstrap migration is not installed")
		}
		if marker != 0 {
			return errors.New("local administrator password was already initialized; refusing to overwrite it")
		}
		var otherUsers int64
		if err := tx.Table("sys_user").Where("id<>1 AND deleted=0").Count(&otherUsers).Error; err != nil {
			return errors.New("unable to check local administrator state")
		}
		if otherUsers != 0 {
			return errors.New("local administrator initialization requires only the seeded admin account")
		}
		var identities int64
		if err := tx.Table("wb_local_identity").Where("id=1").Count(&identities).Error; err != nil {
			return errors.New("unable to check local identity state")
		}
		if identities != 0 {
			return errors.New("local site identity already exists; refusing to initialize administrator bootstrap")
		}
		updated := tx.Table("sys_user").Where("id=1 AND username='admin' AND deleted=0").Update("password", string(hash))
		if updated.Error != nil || updated.RowsAffected != 1 {
			return errors.New("unable to update seeded administrator password")
		}
		if err := tx.Table("auth_session").Where("user_id=1 AND revoked_at IS NULL").Update("revoked_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
			return errors.New("unable to revoke existing administrator sessions")
		}
		if err := tx.Exec("INSERT INTO wb_local_admin_bootstrap (id, initialized_at) VALUES (1, CURRENT_TIMESTAMP)").Error; err != nil {
			return errors.New("unable to record one-time administrator initialization")
		}
		return nil
	})
}
