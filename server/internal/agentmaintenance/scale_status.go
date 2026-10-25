package agentmaintenance

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/agentscale"
)

// ScaleView is read-only. The maintenance service must never open the serial
// port: it only reads the Agent's atomic status file and marks it unavailable
// when the Agent stopped or the file itself is old.
type ScaleView struct {
	Known            bool      `json:"known"`
	Enabled          bool      `json:"enabled"`
	State            string    `json:"state"`
	DeviceOK         bool      `json:"deviceOk"`
	Stable           bool      `json:"stable"`
	InstrumentStable bool      `json:"instrumentStable"`
	Weight           string    `json:"weight"`
	Unit             string    `json:"unit"`
	SourceWeight     string    `json:"sourceWeight"`
	SourceTare       string    `json:"sourceTare"`
	TareIssue        string    `json:"tareIssue,omitempty"`
	WeightType       string    `json:"weightType"`
	Port             string    `json:"port"`
	Baud             int       `json:"baud"`
	Protocol         string    `json:"protocol"`
	LastValidAt      time.Time `json:"lastValidAt"`
	LastFrameAt      time.Time `json:"lastFrameAt"`
	Stale            bool      `json:"stale"`
	StaleMS          int       `json:"staleMs"`
	Fault            string    `json:"fault"`
	Checksum         string    `json:"checksum"`
	ChecksumFailures int       `json:"checksumFailures"`
	FramesValid      uint64    `json:"framesValid"`
	FramesInvalid    uint64    `json:"framesInvalid"`
	DiscardedBytes   uint64    `json:"discardedBytes"`
	UnitFlag         string    `json:"unitFlag"`
	Negative         bool      `json:"negative"`
	Anomaly          bool      `json:"anomaly"`
	WrittenAt        time.Time `json:"writtenAt"`
}

func readScaleView(path string, now time.Time, agentRunning bool) ScaleView {
	b, err := os.ReadFile(path)
	if err != nil {
		return ScaleView{Known: false, State: "未知／尚无仪表状态", Fault: "打印 Agent 尚未写入仪表状态"}
	}
	var file agentscale.StatusFile
	if err := json.Unmarshal(b, &file); err != nil {
		return ScaleView{Known: false, State: "未知／仪表状态不可读取", Fault: "仪表状态文件格式无效"}
	}
	s := file.Snapshot
	v := ScaleView{
		Known: true, Enabled: s.Enabled, DeviceOK: s.DeviceOK, Stable: s.Stable,
		InstrumentStable: s.InstrumentStable,
		Weight:           s.Weight, Unit: s.Unit, SourceWeight: s.SourceWeight, SourceTare: s.SourceTare,
		TareIssue: s.TareIssue, WeightType: s.WeightType, Port: s.Port, Baud: s.Baud,
		Protocol: s.Protocol, LastValidAt: s.LastValidAt, LastFrameAt: s.LastFrameAt,
		Stale: s.Stale, StaleMS: s.StaleMS, Fault: s.Fault, Checksum: s.ChecksumStatus,
		ChecksumFailures: s.ChecksumFailures, FramesValid: s.FramesValid,
		FramesInvalid: s.FramesInvalid, DiscardedBytes: s.DiscardedBytes,
		UnitFlag: s.UnitFlag, Negative: s.Negative, Anomaly: s.Anomaly, WrittenAt: file.At,
	}
	// Serial open failures must name the actual Agent account, not the browser
	// account, and must survive the later stale/stopped early returns.
	if strings.Contains(strings.ToLower(v.Fault), "permission") || strings.Contains(v.Fault, "权限") {
		v.Fault += serialPermissionHint(v.Port)
	}
	// The file itself is a heartbeat. A stopped Agent or a file older than a
	// few seconds cannot be presented as the current weight.
	if file.At.IsZero() || now.Sub(file.At) > 5*time.Second || file.At.Sub(now) > 5*time.Second {
		v.DeviceOK, v.Stable, v.Stale = false, false, true
		v.State = "Agent 状态已过期"
		if v.Fault == "" {
			v.Fault = "仪表状态文件已过期，不能视为当前重量"
		} else {
			v.Fault += "；仪表状态文件已过期"
		}
		return v
	}
	if !agentRunning {
		v.DeviceOK, v.Stable = false, false
		v.State = "Agent 未运行"
		v.Fault = "打印 Agent 未运行；维护服务不读取串口"
		return v
	}
	if !s.Enabled {
		v.DeviceOK, v.Stable = false, false
		v.State = "未配置串口"
		return v
	}
	if s.LastValidAt.IsZero() || now.Sub(s.LastValidAt) > time.Duration(s.StaleMS)*time.Millisecond {
		v.DeviceOK, v.Stable, v.Stale = false, false, true
		v.State = "无新鲜数据"
		if v.Fault == "" {
			v.Fault = fmt.Sprintf("超过 %d ms 未收到有效帧", s.StaleMS)
		}
		return v
	}
	v.Stale = false
	switch {
	case !s.DeviceOK:
		v.State = "故障"
	case s.Stable:
		v.State = "稳定"
	default:
		v.State = "动态"
	}
	return v
}

// serialPermissionHint names the account that actually runs the Agent. A
// browser account is irrelevant to serial permissions, and merely restarting
// the web page cannot grant access to the port.
func serialPermissionHint(port string) string {
	account := "当前 Agent 运行账号"
	uid := os.Geteuid()
	if current, err := user.Current(); err == nil && current.Username != "" {
		account = fmt.Sprintf("%s（UID %s）", current.Username, current.Uid)
	} else {
		account = fmt.Sprintf("UID %d", uid)
	}
	target := "该串口"
	if port != "" {
		target = port
	}
	return fmt.Sprintf("；请管理员为 Agent 运行账号 %s 授予 %s 的读写权限（通常加入 dialout 组），组变更后可能需要重启 Agent 会话才生效", account, target)
}
