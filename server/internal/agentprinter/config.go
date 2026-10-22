package agentprinter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Config is shared by the printer process and its local maintenance service.
type Config struct {
	SiteID       int64   `json:"siteId"`
	SiteCode     string  `json:"siteCode"`
	SiteName     string  `json:"siteName"`
	ScaleCode    string  `json:"scaleCode"`
	ScaleName    string  `json:"scaleName"`
	ServerURL    string  `json:"serverUrl"`
	AgentID      string  `json:"agentId"`
	ScaleID      int64   `json:"scaleId"`
	AgentKey     string  `json:"agentKey"`
	Queue        string  `json:"queue"`
	PrintWidthMM int     `json:"printWidthMm"`
	OffsetXMM    int     `json:"offsetXmm"`
	OffsetYMM    int     `json:"offsetYmm"`
	FormLengthMM float64 `json:"formLengthMm"`
	PrintScale   float64 `json:"printScale"`
}

func Defaults() Config {
	return Config{ServerURL: "http://127.0.0.1:18900", AgentID: "scale-agent-1", ScaleID: 1, Queue: "Aisino-SK820-raw", PrintWidthMM: 175, FormLengthMM: 99, PrintScale: 1}
}

func ValidateConfig(c Config) error {
	if c.ScaleID < 1 || !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`).MatchString(c.AgentID) {
		return errors.New("Agent 编号或地磅编号无效")
	}
	if _, err := CanonicalServerURL(c.ServerURL); err != nil {
		return err
	}
	if strings.ContainsAny(c.AgentKey, "\r\n\x00") {
		return errors.New("Agent 连接密钥格式无效")
	}
	u, e := url.Parse(c.ServerURL)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(c.ServerURL, "\r\n\x00") {
		return errors.New("Server 地址必须为 HTTP(S) URL")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`).MatchString(c.Queue) {
		return errors.New("打印队列名称无效")
	}
	if err := c.Calibration().Validate(); err != nil {
		return err
	}
	return nil
}

// ReadEnvironment parses the shared agent.env without evaluating a shell.
// Defaults are supplied explicitly so both consumers use the same rules.
func ReadEnvironment(path string, cfg Config) (Config, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return cfg, fmt.Errorf("agent.env 格式无效，请使用单行 KEY=VALUE 配置")
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") {
			value, err = strconv.Unquote(value)
			if err != nil {
				return cfg, errors.New("agent.env 引号无效")
			}
		} else if strings.HasPrefix(value, "'") {
			if len(value) < 2 || !strings.HasSuffix(value, "'") {
				return cfg, errors.New("agent.env 引号无效")
			}
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	for _, key := range []string{"WEIGHBRIDGE_SERVER_URL", "WEIGHBRIDGE_AGENT_ID", "WEIGHBRIDGE_SCALE_ID", "WEIGHBRIDGE_PRINT_QUEUE"} {
		if strings.TrimSpace(values[key]) == "" {
			return cfg, fmt.Errorf("agent.env 缺少 %s，请先迁入旧启动参数，避免使用默认身份", key)
		}
	}
	for key, dst := range map[string]*string{"APP_WEIGHING__AGENT_KEY": &cfg.AgentKey, "WEIGHBRIDGE_SERVER_URL": &cfg.ServerURL, "WEIGHBRIDGE_AGENT_ID": &cfg.AgentID, "WEIGHBRIDGE_PRINT_QUEUE": &cfg.Queue} {
		if value, ok := values[key]; ok {
			*dst = value
		}
	}
	if value, ok := values["WEIGHBRIDGE_SCALE_ID"]; ok {
		cfg.ScaleID, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return cfg, errors.New("agent.env 地磅编号无效")
		}
	}
	for key, dst := range map[string]*int{"WEIGHBRIDGE_PRINT_WIDTH_MM": &cfg.PrintWidthMM, "WEIGHBRIDGE_PRINT_OFFSET_X_MM": &cfg.OffsetXMM, "WEIGHBRIDGE_PRINT_OFFSET_Y_MM": &cfg.OffsetYMM} {
		if value, ok := values[key]; ok {
			*dst, err = strconv.Atoi(value)
			if err != nil {
				return cfg, fmt.Errorf("agent.env 参数 %s 无效", key)
			}
		}
	}

	for key, dst := range map[string]*float64{"WEIGHBRIDGE_FORM_LENGTH_MM": &cfg.FormLengthMM, "WEIGHBRIDGE_PRINT_SCALE": &cfg.PrintScale} {
		if value, ok := values[key]; ok {
			*dst, err = strconv.ParseFloat(value, 64)
			if err != nil {
				return cfg, fmt.Errorf("agent.env 参数 %s 无效", key)
			}
		}
	}

	for key, dst := range map[string]*string{"WEIGHBRIDGE_SITE_CODE": &cfg.SiteCode, "WEIGHBRIDGE_SITE_NAME": &cfg.SiteName, "WEIGHBRIDGE_SCALE_CODE": &cfg.ScaleCode, "WEIGHBRIDGE_SCALE_NAME": &cfg.ScaleName} {
		if value, ok := values[key]; ok {
			*dst = value
		}
	}
	if value := values["WEIGHBRIDGE_SITE_ID"]; value != "" {
		cfg.SiteID, err = strconv.ParseInt(value, 10, 64)
		if err != nil || cfg.SiteID < 0 {
			return cfg, errors.New("厂区编号无效")
		}
	}
	cfg = NormalizeConfig(cfg)
	return cfg, ValidateConfig(cfg)
}

// ConfigDigest lets maintenance compare the running process with agent.env
// without exposing the connection key in heartbeat files or API responses.
func ConfigDigest(c Config) string {
	data, _ := json.Marshal(c)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// NormalizeConfig keeps pre-scaling maintenance files equivalent to 100% scale.
func NormalizeConfig(c Config) Config {
	if c.PrintScale == 0 {
		c.PrintScale = 1
	}
	return c
}

// Calibration is the single local configuration mapping used by both print paths.
func (c Config) Calibration() Calibration {
	c = NormalizeConfig(c)
	return Calibration{WidthMM: c.PrintWidthMM, OffsetXMM: c.OffsetXMM, OffsetYMM: c.OffsetYMM, FormLengthMM: c.FormLengthMM, Scale: c.PrintScale}
}
