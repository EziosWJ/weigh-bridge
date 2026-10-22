package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/EziosWJ/weighbridge/server/internal/agentprinter"
)

// Managed deployments always read the same agent.env used by maintenance.
// Legacy flags remain available for standalone diagnostic launches only.
func printerOptions(args []string, getenv func(string) string) (agentprinter.Config, string, error) {
	c := agentprinter.Defaults()
	f := flag.NewFlagSet("agent-printer", flag.ContinueOnError)
	file := f.String("env-file", "", "shared agent.env; cannot combine with connection flags")
	state := f.String("state-dir", "./state", "Durable attempt directory")
	f.StringVar(&c.ServerURL, "server", c.ServerURL, "Server URL (standalone)")
	f.StringVar(&c.AgentID, "agent-id", c.AgentID, "Agent identity (standalone)")
	f.Int64Var(&c.ScaleID, "scale-id", c.ScaleID, "Scale identity (standalone)")
	f.StringVar(&c.Queue, "queue", c.Queue, "CUPS queue (standalone)")
	if err := f.Parse(args); err != nil {
		return c, *state, err
	}
	if *file != "" {
		conflict := false
		f.Visit(func(v *flag.Flag) {
			if v.Name != "env-file" && v.Name != "state-dir" {
				conflict = true
			}
		})
		if conflict {
			return c, *state, errors.New("使用 --env-file 时请将连接和队列参数迁入 agent.env")
		}
		if _, err := os.Stat(*file); err != nil {
			return c, *state, fmt.Errorf("读取 agent.env 失败：%w", err)
		}
		var err error
		c, err = agentprinter.ReadEnvironment(*file, c)
		if err != nil {
			return c, *state, err
		}
	} else {
		for key, dst := range map[string]*string{
			"WEIGHBRIDGE_SERVER_URL": &c.ServerURL, "WEIGHBRIDGE_AGENT_ID": &c.AgentID,
			"WEIGHBRIDGE_PRINT_QUEUE": &c.Queue, "APP_WEIGHING__AGENT_KEY": &c.AgentKey,
		} {
			if value := getenv(key); value != "" {
				*dst = value
			}
		}
		if value := getenv("WEIGHBRIDGE_SCALE_ID"); value != "" {
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return c, *state, errors.New("WEIGHBRIDGE_SCALE_ID 无效")
			}
			c.ScaleID = n
		}
		for key, dst := range map[string]*int{
			"WEIGHBRIDGE_PRINT_WIDTH_MM": &c.PrintWidthMM, "WEIGHBRIDGE_PRINT_OFFSET_X_MM": &c.OffsetXMM,
			"WEIGHBRIDGE_PRINT_OFFSET_Y_MM": &c.OffsetYMM,
		} {
			if value := getenv(key); value != "" {
				n, err := strconv.Atoi(value)
				if err != nil {
					return c, *state, fmt.Errorf("%s 无效", key)
				}
				*dst = n
			}
		}
		for key, dst := range map[string]*float64{"WEIGHBRIDGE_FORM_LENGTH_MM": &c.FormLengthMM, "WEIGHBRIDGE_PRINT_SCALE": &c.PrintScale} {
			if value := getenv(key); value != "" {
				n, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return c, *state, fmt.Errorf("%s 无效", key)
				}
				*dst = n
			}
		}
	}
	if c.AgentKey == "" {
		return c, *state, errors.New("Agent 连接密钥尚未设置")
	}
	c = agentprinter.NormalizeConfig(c)
	return c, *state, agentprinter.ValidateConfig(c)
}
