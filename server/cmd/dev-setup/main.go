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
	"github.com/knadh/koanf/parsers/yaml"
)

const (
	devConfigName   = "config.dev.yaml"
	devTemplateName = "config.dev.example.yaml"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "init" && args[0] != "doctor") {
		return errors.New("用法：dev-setup init|doctor [--database postgres]")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	driver := flags.String("database", "", "postgres")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(out, "用法：dev-setup init|doctor [--database postgres]")
			return nil
		}
		return errors.New("参数无效；使用 --database postgres")
	}
	if flags.NArg() != 0 || (*driver != "" && *driver != "postgres") {
		return errors.New("参数无效；使用 --database postgres")
	}
	if args[0] == "init" {
		if *driver == "" {
			*driver = "postgres"
		}
		return initialize(*driver, out)
	}

	return diagnose(ctx, *driver, out)
}

func initialize(driver string, out io.Writer) error {
	path := filepath.Join(config.DefaultDir, devConfigName)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode().IsRegular() {
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return errors.New("无法读取已有开发配置")
			}
			updated, changed, maintenanceErr := withMaintenanceDirectory(string(contents))
			if maintenanceErr != nil {
				return maintenanceErr
			}
			if changed {
				// Replace atomically; do not risk truncating existing credentials.
				temporary, createErr := os.CreateTemp(filepath.Dir(path), ".config.dev.maintenance-*")
				if createErr != nil {
					return errors.New("无法补充维护目录配置")
				}
				defer os.Remove(temporary.Name())
				_, writeErr := io.WriteString(temporary, updated)
				closeErr := temporary.Close()
				if writeErr != nil || closeErr != nil {
					return errors.New("无法写入维护目录配置")
				}
				if err := os.Rename(temporary.Name(), path); err != nil {
					return errors.New("无法保存维护目录配置")
				}
				fmt.Fprintln(out, "[已补充] 缺失的维护目录；已有 JWT、数据库与其他配置保留。请重启 API 读取配置。")
			}
		}
		fmt.Fprintln(out, "[保留] config.dev.yaml 已存在；已有配置与 JWT 保留（仅补齐缺失项）。")
		fmt.Fprintln(out, "下一步：task doctor")
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
		fmt.Fprintln(out, "[保留] config.dev.yaml 已存在；已有配置与 JWT 保留（仅补齐缺失项）。")
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
	fmt.Fprintln(out, "[已生成] config.dev.yaml，包含独立的随机 JWT、Agent 连接密钥、本机上传及维护目录。")
	if driver == database.DriverPostgres {
		fmt.Fprintln(out, "先填写 database.url、database.username、database.password，并准备对应 PostgreSQL 数据库；不会自动创建数据库或启动 Docker。")
	}
	fmt.Fprintln(out, "下一步：task doctor；检查完成后运行 task dev")
	return nil
}

// render fills the versioned template with the values that cannot be shared
// between checkouts. The template stays authoritative for structure and
// comments: every comment and blank line is copied through verbatim, and only
// the replaced value lines differ from it. Substitution is keyed on the section
// and field names rather than on template values, so editing a placeholder does
// not silently stop the substitution.
func render(template, driver, uploads, secret string) (string, error) {
	if driver != database.DriverPostgres {
		return "", errors.New("参数无效；使用 --database postgres")
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
			case "maintenance.enabled":
				line = "  enabled: true"

			}
			filled[field] = true
		}
		out = append(out, line)
	}
	required := []string{"jwt.secret", "file.storage_root"}

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
	fmt.Fprintln(out, "[通过] 开发配置（数据库："+cfg.Database.Driver+"；已应用 APP_ 环境变量覆盖）")
	startTask := "task dev"
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := database.CheckConnection(checkCtx, cfg.Database); err != nil {
		return errors.New("[失败] PostgreSQL 未就绪；确认目标数据库已创建、服务可达、database.url 与用户名密码正确。检查不会创建数据库或启动 Docker")
	}
	if cfg.Maintenance.DataDir == "" {
		fmt.Fprintln(out, "[提示] 设备维护未启用；默认开发配置运行 task init 补齐维护目录后重启 API。")
	} else {
		fmt.Fprintln(out, "[通过] 已配置独立维护目录；API 启动后按权限提供设备维护，不自动应用配置或启动设备。")
	}
	fmt.Fprintln(out, "[通过] 数据库连接（未执行迁移；此检查不代表 schema 已更新）")
	fmt.Fprintln(out, "下一步："+startTask+"；启动后检查 /health 和 /ready")
	return nil
}

// Only backfill absent development settings. An explicit empty directory keeps
// maintenance disabled.
func withMaintenanceDirectory(contents string) (string, bool, error) {
	document, err := yaml.Parser().Unmarshal([]byte(contents))
	if err != nil {
		return contents, false, nil
	}
	section, ok := document["maintenance"].(map[string]any)
	if document["maintenance"] != nil && !ok {
		return contents, false, nil
	}
	if _, found := section["enabled"]; found {
		return contents, false, nil
	}
	if _, found := section["data_dir"]; found {
		return contents, false, nil
	}
	if section == nil {
		section = map[string]any{}
	}
	encoded, err := yaml.Parser().Marshal(map[string]any{"maintenance": map[string]any{"enabled": true}})
	if err != nil {
		return "", false, err
	}
	lines := strings.Split(contents, "\n")
	for i, line := range lines {
		if !regexp.MustCompile(`^maintenance\s*:`).MatchString(line) {
			continue
		}
		remainder := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		if remainder != "" && !strings.HasPrefix(remainder, "#") {
			section["enabled"] = true
			encoded, err = yaml.Parser().Marshal(map[string]any{"maintenance": section})
			if err != nil {
				return "", false, err
			}
			lines[i] = strings.TrimSuffix(string(encoded), "\n")
		} else {
			value, err := yaml.Parser().Marshal(map[string]any{"enabled": true})
			if err != nil {
				return "", false, err
			}
			lines[i] += "\n  " + strings.TrimSuffix(string(value), "\n")
		}
		return strings.Join(lines, "\n"), true, nil
	}
	return strings.TrimRight(contents, "\n") + "\n\n" + string(encoded), true, nil
}
