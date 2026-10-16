// Command dev-setup prepares and diagnoses local development configuration.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/config"
	"github.com/EziosWJ/weighbridge/server/internal/platform/database"
)

const (
	devConfigName   = "config.dev.yaml"
	devTemplateName = "config.dev.example.yaml"
	// sqliteConfigURL mirrors the path used by the config.sqlite.yaml profile.
	sqliteConfigURL = ".data/weighbridge.db"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "init" && args[0] != "doctor") {
		return errors.New("用法：dev-setup init|doctor [--database postgres|sqlite]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	driver := flags.String("database", "", "postgres or sqlite")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(out, "用法：dev-setup init|doctor [--database postgres|sqlite]")
			return nil
		}
		return errors.New("参数无效；使用 --database postgres 或 --database sqlite")
	}
	if flags.NArg() != 0 || (*driver != "" && *driver != "postgres" && *driver != "sqlite") {
		return errors.New("参数无效；使用 --database postgres 或 --database sqlite")
	}
	if args[0] == "init" {
		if *driver == "" {
			*driver = "postgres"
		}
		return initialize(*driver, out)
	}
	if *driver != "" {
		profile := ""
		if *driver == "sqlite" {
			profile = "sqlite"
		}
		if err := os.Setenv("APP_CONFIG_PROFILE", profile); err != nil {
			return errors.New("无法选择数据库 profile")
		}
	}
	return diagnose(ctx, *driver, out)
}

func initialize(driver string, out io.Writer) error {
	path := filepath.Join(config.DefaultDir, devConfigName)
	if _, err := os.Lstat(path); err == nil {
		fmt.Fprintln(out, "[保留] config.dev.yaml 已存在；原配置与 JWT 未修改。")
		fmt.Fprintln(out, "下一步：task doctor（使用 SQLite profile 时加 -- --database sqlite）")
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("无法检查已有开发配置；检查 configs 目录访问权限")
	}
	template, err := os.ReadFile(filepath.Join(config.DefaultDir, devTemplateName))
	if err != nil {
		return errors.New("无法读取开发配置模板；检查 configs/config.dev.example.yaml")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		return errors.New("无法确定项目目录")
	}
	// storage_root is derived from the working directory rather than from the
	// template, so an unexpected working directory would silently point the
	// checkout at a different tree. Confirm the template is reachable from the
	// resolved root before writing anything.
	if _, err := os.Stat(filepath.Join(root, "server", config.DefaultDir, devTemplateName)); err != nil {
		return errors.New("无法定位项目根目录；请在 server 目录内运行，或改用 task init")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return errors.New("无法生成 JWT 随机密钥")
	}
	contents, err := render(string(template), driver, filepath.Join(root, ".data", "uploads"), hex.EncodeToString(secret))
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		fmt.Fprintln(out, "[保留] config.dev.yaml 已存在；原配置与 JWT 未修改。")
		return nil
	}
	if err != nil {
		return errors.New("无法创建开发配置；检查 configs 目录写权限")
	}
	_, writeErr := io.WriteString(file, contents)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		return errors.New("开发配置写入失败；检查磁盘空间和目录权限后重试")
	}
	fmt.Fprintln(out, "[已生成] config.dev.yaml，包含新的随机 JWT 和本机上传目录。")
	if driver == database.DriverPostgres {
		fmt.Fprintln(out, "先填写 database.url、database.username、database.password，并准备对应 PostgreSQL 数据库；不会自动创建数据库或启动 Docker。")
	}
	doctorTask := "task doctor"
	if driver == database.DriverSQLite {
		doctorTask += " -- --database sqlite"
	}
	fmt.Fprintln(out, "下一步："+doctorTask+"；检查完成后运行 "+startupTask(driver))
	return nil
}

// render fills the versioned template with the values that cannot be shared
// between checkouts. The template stays authoritative for structure and
// comments: every comment and blank line is copied through verbatim, and only
// the replaced value lines differ from it. Substitution is keyed on the section
// and field names rather than on template values, so editing a placeholder does
// not silently stop the substitution.
func render(template, driver, uploads, secret string) (string, error) {
	sqlite := driver == database.DriverSQLite
	if !sqlite && driver != database.DriverPostgres {
		return "", errors.New("参数无效；使用 --database postgres 或 --database sqlite")
	}
	filled := make(map[string]bool)
	section := ""
	lines := strings.Split(strings.TrimSuffix(template, "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			// Comments and blank lines are preserved as written.
		case !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":"):
			section = strings.TrimSuffix(trimmed, ":")
		default:
			key, _, found := strings.Cut(trimmed, ":")
			if !found {
				return "", errors.New("开发配置模板格式无效；检查 configs/config.dev.example.yaml")
			}
			field := section + "." + strings.TrimSpace(key)
			switch field {
			case "jwt.secret":
				line = "  secret: " + secret
			case "file.storage_root":
				line = "  storage_root: " + uploads
			case "database.driver":
				if sqlite {
					line = "  driver: " + driver
				}
			case "database.url":
				if sqlite {
					line = "  url: " + sqliteConfigURL
				}
			case "database.username", "database.password":
				if sqlite {
					// SQLite carries no credentials; the loader rejects them.
					continue
				}
			}
			filled[field] = true
		}
		out = append(out, line)
	}
	required := []string{"jwt.secret", "file.storage_root"}
	if sqlite {
		required = append(required, "database.driver", "database.url")
	}
	for _, field := range required {
		if !filled[field] {
			return "", errors.New("开发配置模板缺少 " + field + "；检查 configs/config.dev.example.yaml")
		}
	}
	return strings.Join(out, "\n") + "\n", nil
}

func diagnose(ctx context.Context, selectedDriver string, out io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		// YAML/type errors can echo values. Only report known field names, never
		// the original error, database URL, password or JWT secret.
		fields := regexp.MustCompile(`\b(?:database|jwt|file|http|auth|monitoring|cors|log|service|swagger)\.[a-z_]+|APP_ENV|APP_CONFIG_PROFILE`).FindAllString(err.Error(), -1)
		fmt.Fprintln(out, "[失败] 配置无效；检查 YAML 格式、数据库配置、JWT 和绝对上传目录，以及 APP_ 环境变量覆盖。")
		if len(fields) != 0 {
			fmt.Fprintln(out, "检查字段："+strings.Join(fields, ", "))
		}
		return errors.New("修复开发配置后重新运行 task doctor；首次使用请先运行 task init")
	}
	if cfg.Environment != config.EnvironmentDev {
		return errors.New("本命令只检查开发环境；请使用 APP_ENV=dev")
	}
	if selectedDriver != "" && selectedDriver != cfg.Database.Driver {
		return errors.New("所选数据库与有效配置不一致；检查开发配置和 APP_ 环境变量覆盖")
	}
	fmt.Fprintln(out, "[通过] 开发配置（数据库："+cfg.Database.Driver+"；已应用 APP_ 环境变量与 profile 覆盖）")
	// Match the selected profile, not just the driver: using dev:sqlite for a
	// custom SQLite path without that profile would check and start different DBs.
	startTask := "task dev"
	if os.Getenv("APP_CONFIG_PROFILE") == "sqlite" {
		startTask = "task dev:sqlite"
	}
	if cfg.Database.Driver == database.DriverSQLite {
		if _, err := os.Stat(cfg.Database.URL); errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(out, "[待迁移] SQLite 文件尚未创建；运行 "+startTask+" 执行显式迁移并启动。检查未创建数据库。")
			return nil
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := database.CheckConnection(checkCtx, cfg.Database); err != nil {
		if cfg.Database.Driver == database.DriverPostgres {
			return errors.New("[失败] PostgreSQL 未就绪；确认目标数据库已创建、服务可达、database.url 与用户名密码正确。检查不会创建数据库或启动 Docker")
		}
		return errors.New("[失败] SQLite 文件不可读取；检查路径、访问权限及数据库文件是否有效")
	}
	fmt.Fprintln(out, "[通过] 数据库连接（未执行迁移；此检查不代表 schema 已更新）")
	fmt.Fprintln(out, "下一步："+startTask+"；启动后检查 /health 和 /ready")
	return nil
}

func startupTask(driver string) string {
	if driver == database.DriverSQLite {
		return "task dev:sqlite"
	}
	return "task dev"
}
