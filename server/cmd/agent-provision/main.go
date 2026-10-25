// Command agent-provision writes the Agent connection file without requiring
// database access. ADR 0030: only the Server address is needed; site, scale and
// Agent identities are assigned by the Server during the hello handshake.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

const generatedHeader = `# 由 weighbridge-agent-provision 生成。Agent 只要求可访问的 WEIGHBRIDGE_SERVER_URL；
# 串口与打印参数可在 Agent 维护页保存并应用。摄像头和视频服务配置在 Server 摄像头页维护。
`

// deprecatedKeys are removed from the Agent environment. Camera values are
// captured into a separate legacy backup before removal so that upgrade never
// silently loses a real source address that still has to be re-entered in the
// Server camera page.
var deprecatedKeys = map[string]bool{
	"APP_WEIGHING__AGENT_KEY":           true,
	"WEIGHBRIDGE_AGENT_KEY":             true,
	"WEIGHBRIDGE_AGENT_ID":              true,
	"WEIGHBRIDGE_SCALE_ID":              true,
	"WEIGHBRIDGE_SITE_ID":               true,
	"WEIGHBRIDGE_SITE_CODE":             true,
	"WEIGHBRIDGE_SITE_NAME":             true,
	"WEIGHBRIDGE_SCALE_CODE":            true,
	"WEIGHBRIDGE_SCALE_NAME":            true,
	"WEIGHBRIDGE_SERIAL_ENABLED":        true,
	"WEIGHBRIDGE_SERIAL_PROTOCOL":       true,
	"WEIGHBRIDGE_SERIAL_WEIGHT_UNIT":    true,
	"WEIGHBRIDGE_SERIAL_STALE_MS":       true,
	"WEIGHBRIDGE_PLATE_RECOGNITION_URL": true,
}

var orderedKeys = []string{"WEIGHBRIDGE_SERVER_URL", "WEIGHBRIDGE_PRINT_QUEUE", "WEIGHBRIDGE_SERIAL_PORT", "WEIGHBRIDGE_SERIAL_BAUD"}

type options struct {
	ServerURL  string
	PrintQueue string
	SerialPort string
	SerialBaud int
	Update     bool
}

type cameraBackup struct {
	MigratedAt string             `json:"migratedAt"`
	Note       string             `json:"note"`
	Entries    []cameraBackupItem `json:"entries"`
}

type cameraBackupItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func main() {
	var configDir, envPath, serverURL, printQueue, serialPort string
	var serialBaud int
	var updateConnection bool
	// config-dir is retained only so old deployment scripts keep working. The
	// tool never opens the database: an Agent-only host has no database
	// credentials and must be provisionable from the Server address alone.
	flag.StringVar(&configDir, "config-dir", "", "deprecated; accepted for compatibility and ignored")
	flag.StringVar(&envPath, "agent-env-path", "", "target agent.env path")
	flag.StringVar(&serverURL, "server-url", "", "business service base URL reachable from the Agent host, e.g. http://127.0.0.1:18900")
	flag.BoolVar(&updateConnection, "update-connection", false, "explicitly update an existing agent.env, preserving device settings")
	flag.StringVar(&printQueue, "print-queue", "", "CUPS queue for a new environment file")
	flag.StringVar(&serialPort, "serial-port", "", "D12 serial device path for a new environment file")
	flag.IntVar(&serialBaud, "serial-baud", 0, "D12 serial baud rate for a new environment file")
	flag.Parse()
	if flag.NArg() != 0 || envPath == "" {
		flag.Usage()
		os.Exit(2)
	}
	if configDir != "" {
		fmt.Fprintln(os.Stderr, "提示：--config-dir 已废弃，agent-provision 不再连接业务数据库。")
	}
	err := run(envPath, options{ServerURL: serverURL, PrintQueue: printQueue, SerialPort: serialPort, SerialBaud: serialBaud, Update: updateConnection})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Agent 连接配置已写入（权限 0600）；已有串口、打印参数和打印账本未更改。")
}

func run(envPath string, opts options) error {
	exists := false
	if info, err := os.Lstat(envPath); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("目标 agent.env 必须是普通文件")
		}
		exists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("无法检查目标 agent.env")
	}
	if exists && !opts.Update {
		return errors.New("agent.env 已存在；如需更新连接地址并清理废弃字段，请显式传入 --update-connection")
	}
	var contents string
	if exists {
		b, err := os.ReadFile(envPath)
		if err != nil {
			return errors.New("无法读取已有 Agent 配置；文件未更改")
		}
		contents = string(b)
	}
	lines := parseLines(contents)
	current := valuesOf(lines)
	serverURL, err := resolveServerURL(opts.ServerURL, current["WEIGHBRIDGE_SERVER_URL"], exists)
	if err != nil {
		return err
	}

	camera := collectCamera(current)
	desired := map[string]string{"WEIGHBRIDGE_SERVER_URL": strconv.Quote(serverURL)}
	if !exists {
		defaults := agentprinter.Defaults()
		queue := opts.PrintQueue
		if queue == "" {
			queue = defaults.Queue
		}
		port := opts.SerialPort
		baud := opts.SerialBaud
		if baud == 0 {
			baud = defaults.SerialBaud
		}
		desired["WEIGHBRIDGE_PRINT_QUEUE"] = strconv.Quote(queue)
		desired["WEIGHBRIDGE_SERIAL_PORT"] = strconv.Quote(port)
		desired["WEIGHBRIDGE_SERIAL_BAUD"] = strconv.Itoa(baud)
	} else {
		if opts.PrintQueue != "" {
			desired["WEIGHBRIDGE_PRINT_QUEUE"] = strconv.Quote(opts.PrintQueue)
		}
		if opts.SerialPort != "" {
			desired["WEIGHBRIDGE_SERIAL_PORT"] = strconv.Quote(opts.SerialPort)
		}
		if opts.SerialBaud != 0 {
			desired["WEIGHBRIDGE_SERIAL_BAUD"] = strconv.Itoa(opts.SerialBaud)
		}
	}
	updated := rewriteEnvironment(lines, desired, !exists)
	if err := os.MkdirAll(filepath.Dir(envPath), 0o700); err != nil {
		return errors.New("无法准备 Agent 配置目录")
	}
	if err := validateResult(envPath, updated); err != nil {
		return err
	}
	if err := writePrivate(envPath, []byte(updated)); err != nil {
		return errors.New("无法安全写入 Agent 连接配置；敏感信息未输出")
	}
	if len(camera) > 0 {
		if err := writeCameraBackup(envPath+".legacy-camera.json", camera); err != nil {
			return errors.New("Agent 连接配置已更新，但旧摄像头配置备份失败；请先手工记录源地址再重试")
		}
		fmt.Fprintf(os.Stderr, "旧摄像头配置已保存到 %s.legacy-camera.json（仅本机可读），请在 Server 摄像头页重新填写。\n", envPath)
	}
	return nil
}

type envLine struct {
	raw   string
	key   string
	value string
	isKV  bool
}

func parseLines(contents string) []envLine {
	trimmed := strings.TrimSuffix(contents, "\n")
	if trimmed == "" {
		return nil
	}
	rawLines := strings.Split(trimmed, "\n")
	out := make([]envLine, 0, len(rawLines))
	for _, raw := range rawLines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			out = append(out, envLine{raw: raw})
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			out = append(out, envLine{raw: raw})
			continue
		}
		key = strings.TrimSpace(key)
		out = append(out, envLine{raw: raw, key: key, value: strings.TrimSpace(value), isKV: true})
	}
	return out
}

func valuesOf(lines []envLine) map[string]string {
	values := make(map[string]string, len(lines))
	for _, line := range lines {
		if line.isKV {
			values[line.key] = unquote(line.value)
		}
	}
	return values
}

func unquote(value string) string {
	if strings.HasPrefix(value, "\"") {
		if parsed, err := strconv.Unquote(value); err == nil {
			return parsed
		}
	}
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return value[1 : len(value)-1]
	}
	return value
}

func rewriteEnvironment(lines []envLine, desired map[string]string, fresh bool) string {
	seen := make(map[string]bool, len(desired))
	out := make([]string, 0, len(lines)+len(desired)+2)
	if fresh {
		out = append(out, strings.TrimSuffix(generatedHeader, "\n"))
	}
	for _, line := range lines {
		if !line.isKV {
			out = append(out, line.raw)
			continue
		}
		if deprecatedKeys[line.key] || strings.HasPrefix(line.key, "WEIGHBRIDGE_CAMERA_") {
			continue
		}
		if value, ok := desired[line.key]; ok {
			out = append(out, line.key+"="+value)
			seen[line.key] = true
			continue
		}
		out = append(out, line.raw)
	}
	for _, key := range orderedKeys {
		if value, ok := desired[key]; ok && !seen[key] {
			out = append(out, key+"="+value)
		}
	}
	return strings.Join(out, "\n") + "\n"
}

func collectCamera(values map[string]string) []cameraBackupItem {
	var items []cameraBackupItem
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.HasPrefix(key, "WEIGHBRIDGE_CAMERA_") || key == "WEIGHBRIDGE_PLATE_RECOGNITION_URL" {
			items = append(items, cameraBackupItem{Key: key, Value: value})
		}
	}
	// Keep output deterministic for tests and operators.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j-1].Key > items[j].Key; j-- {
			items[j-1], items[j] = items[j], items[j-1]
		}
	}
	return items
}

func writeCameraBackup(path string, items []cameraBackupItem) error {
	existing := cameraBackup{Note: "摄像头配置已迁入 Server；请在本厂摄像头页重新填写源地址和凭据。", Entries: []cameraBackupItem{}}
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &existing) != nil {
			existing = cameraBackup{Note: "摄像头配置已迁入 Server；请在本厂摄像头页重新填写源地址和凭据。", Entries: []cameraBackupItem{}}
		}
	}
	seen := make(map[string]bool, len(existing.Entries))
	for _, item := range existing.Entries {
		seen[item.Key] = true
	}
	for _, item := range items {
		if !seen[item.Key] {
			existing.Entries = append(existing.Entries, item)
			seen[item.Key] = true
		}
	}
	existing.MigratedAt = time.Now().UTC().Format(time.RFC3339)
	existing.Note = "摄像头配置已迁入 Server；请在本厂摄像头页重新填写源地址和凭据。"
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return writePrivate(path, append(data, '\n'))
}

// validateResult ensures the rewritten file is still a usable Agent
// environment before replacing the previous file.
func validateResult(path, contents string) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".agent-validate-*")
	if err != nil {
		return errors.New("无法校验 Agent 配置")
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return errors.New("无法校验 Agent 配置")
	}
	if _, err := temp.WriteString(contents); err != nil {
		_ = temp.Close()
		return errors.New("无法校验 Agent 配置")
	}
	if err := temp.Close(); err != nil {
		return errors.New("无法校验 Agent 配置")
	}
	if _, err := agentprinter.ReadEnvironment(name, agentprinter.Defaults()); err != nil {
		return fmt.Errorf("生成的 Agent 配置无效：%w", err)
	}
	return nil
}

func writePrivate(path string, contents []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".agent-provision-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// resolveServerURL requires an explicit address for a new file. Updating an
// existing file without the flag keeps its already valid address.
func resolveServerURL(explicit, existing string, exists bool) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		url, err := agentprinter.CanonicalServerURL(value)
		if err != nil {
			return "", errors.New("请提供有效的 Agent 连接业务服务地址，例如 http://127.0.0.1:18900")
		}
		return url, nil
	}
	if exists {
		if url, err := agentprinter.CanonicalServerURL(existing); err == nil {
			return url, nil
		}
	}
	return "", errors.New("请显式提供 --server-url（Agent 主机可访问的业务服务地址），例如 http://127.0.0.1:18900")
}
