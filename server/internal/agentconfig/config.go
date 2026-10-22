// Package agentconfig describes the portable connection settings for one site.
package agentconfig

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

type Connection struct {
	ServerURL string `json:"serverUrl"`
	SiteID    int64  `json:"siteId"`
	SiteCode  string `json:"siteCode"`
	SiteName  string `json:"siteName"`
	ScaleID   int64  `json:"scaleId"`
	ScaleCode string `json:"scaleCode"`
	ScaleName string `json:"scaleName"`
	AgentID   string `json:"agentId"`
	AgentKey  string `json:"agentKey"`
}

func (c Connection) Validate() error {
	if c.SiteID < 1 || c.ScaleID < 1 || strings.TrimSpace(c.SiteName) == "" || !regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`).MatchString(c.AgentID) || strings.TrimSpace(c.AgentKey) == "" || strings.ContainsAny(c.AgentKey, "\r\n\x00") {
		return errors.New("厂区启动配置不完整或格式无效")
	}
	_, err := agentprinter.CanonicalServerURL(c.ServerURL)
	return err
}
func Read(path string) (Connection, error) {
	var c Connection
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if len(b) > 16384 {
		return c, errors.New("厂区配置文件过大")
	}
	if json.Unmarshal(b, &c) != nil {
		return c, errors.New("厂区配置必须为有效 JSON 文件")
	}
	return c, c.Validate()
}
