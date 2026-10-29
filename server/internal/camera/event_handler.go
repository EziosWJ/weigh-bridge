package camera

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/rbac"
	"github.com/gin-gonic/gin"
)

const eventBodyLimit = 15 << 20

var eventSlots = make(chan struct{}, 8)

// RegisterEventRoute is intentionally outside the user-authenticated router.
// Network access must be restricted to the trusted device network.
func RegisterEventRoute(router gin.IRouter, service *Service) {
	allowed := os.Getenv("WEIGHBRIDGE_CAMERA_EVENT_ALLOWED_IPS")
	strategyEnabled := os.Getenv("WEIGHBRIDGE_CAMERA_HEARTBEAT_STRATEGY") == "1"
	router.POST("/api/v1/event/pull", func(c *gin.Context) {
		if !eventSourceAllowed(allowed, c.Request.RemoteAddr) {
			c.Status(http.StatusForbidden)
			return
		}
		select {
		case eventSlots <- struct{}{}:
			defer func() { <-eventSlots }()
		default:
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, eventBodyLimit)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				c.Status(http.StatusRequestEntityTooLarge)
			} else {
				c.Status(http.StatusBadRequest)
			}
			return
		}
		var envelope struct {
			Operator   string `json:"operator"`
			DeviceInfo struct {
				SerialNumber string `json:"serialNumber"`
				IP           string `json:"IP"`
			} `json:"deviceInfo"`
			Info struct {
				EventID json.RawMessage `json:"eventId"`
				Time    string          `json:"time"`
			} `json:"info"`
		}
		if json.Unmarshal(body, &envelope) != nil || !validEventText(envelope.Operator, 200) || !validEventText(envelope.DeviceInfo.SerialNumber, 200) || !validEventText(envelope.Info.Time, 200) || len(envelope.DeviceInfo.IP) > 200 || !validEventID(envelope.Info.EventID) {
			c.Status(http.StatusBadRequest)
			return
		}
		if envelope.Operator == "CarLicenseSnapshot" {
			sum := sha256.Sum256(body)
			hash := hex.EncodeToString(sum[:])
			started := time.Now()
			ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
			defer cancel()
			accepted, err := service.receiveSnapshot(ctx, envelope.DeviceInfo.SerialNumber, hash, body, time.Now().UTC())
			if err != nil {
				reason := "storage"
				if errors.Is(err, ErrInvalid) {
					reason = err.Error()
				}
				if errors.Is(err, ErrNotFound) {
					reason = "unknown device"
				}
				slog.Warn("camera snapshot receive failed", "reason", reason, "bytes", len(body), "sha256", hash)
				if store, ok := service.store.(eventStore); ok && !errors.Is(err, ErrNotFound) {
					category := "storage"
					if errors.Is(err, ErrInvalid) {
						category = "invalid"
					}
					diagnostic, _ := json.Marshal(map[string]string{"failure": category})
					diagnosticCtx, stop := context.WithTimeout(c.Request.Context(), 5*time.Second)
					_ = store.ReceiveEvent(diagnosticCtx, envelope.DeviceInfo.SerialNumber, envelope.Operator, string(envelope.Info.EventID), envelope.Info.Time, envelope.DeviceInfo.IP, time.Now().UTC(), hash, diagnostic)
					stop()
				}
				switch {
				case errors.Is(err, ErrNotFound):
					c.Status(http.StatusForbidden)
				case errors.Is(err, ErrInvalid):
					c.Status(http.StatusBadRequest)
				default:
					c.Status(http.StatusServiceUnavailable)
				}
				return
			}
			ack, err := json.Marshal(struct {
				Operator string `json:"operator"`
				Info     struct {
					EventID json.RawMessage `json:"eventID"`
				} `json:"info"`
				Result struct {
					ErrorNo     int    `json:"errorNo"`
					Description string `json:"description"`
				} `json:"result"`
			}{Operator: "CarLicenseSnapshot-Ack", Info: struct {
				EventID json.RawMessage `json:"eventID"`
			}{envelope.Info.EventID}, Result: struct {
				ErrorNo     int    `json:"errorNo"`
				Description string `json:"description"`
			}{0, "ok"}})
			if err != nil {
				c.Status(http.StatusInternalServerError)
				return
			}
			if accepted {
				slog.Info("camera snapshot accepted", "bytes", len(body), "sha256", hash, "duration_ms", time.Since(started).Milliseconds())
			} else {
				slog.Info("camera snapshot expired", "bytes", len(body), "sha256", hash, "duration_ms", time.Since(started).Milliseconds())
			}
			c.Data(http.StatusOK, "application/json", ack)
			return
		}
		store, ok := service.store.(eventStore)
		if !ok {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		now := time.Now().UTC()
		var diagnostic json.RawMessage
		hash := ""
		if envelope.Operator != "heartbeat" {
			var raw any
			if json.Unmarshal(body, &raw) != nil {
				c.Status(http.StatusBadRequest)
				return
			}
			diagnostic, err = json.Marshal(redactEvent(raw, 0))
			if err != nil || len(diagnostic) > 32768 {
				c.Status(http.StatusBadRequest)
				return
			}
			sum := sha256.Sum256(body)
			hash = hex.EncodeToString(sum[:])
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		err = store.ReceiveEvent(ctx, envelope.DeviceInfo.SerialNumber, envelope.Operator, string(envelope.Info.EventID), envelope.Info.Time, envelope.DeviceInfo.IP, now, hash, diagnostic)
		if err != nil {
			slog.Warn("camera event receive failed", "unknown_device", errors.Is(err, ErrNotFound))
			if errors.Is(err, ErrNotFound) {
				c.Status(http.StatusForbidden)
			} else {
				c.Status(http.StatusServiceUnavailable)
			}
			return
		}
		if envelope.Operator != "heartbeat" { // Manufacturer event ACK is not yet known.
			c.Status(http.StatusServiceUnavailable)
			return
		}
		request, err := decodeHeartbeat(body)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		ack, err := heartbeatACK(request, now, strategyEnabled)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Data(http.StatusOK, "application/json", ack)
	})
}

// Optional direct-peer allowlist; never trust forwarded headers for devices.
// An unset value preserves the network-rule deployment mode.
func eventSourceAllowed(allowed, remote string) bool {
	if strings.TrimSpace(allowed) == "" {
		return true
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	peer := net.ParseIP(host)
	if peer == nil {
		return false
	}
	for _, part := range strings.Split(allowed, ",") {
		if ip := net.ParseIP(strings.TrimSpace(part)); ip != nil && ip.Equal(peer) {
			return true
		}
	}
	return false
}
func validEventText(s string, max int) bool {
	return s != "" && strings.TrimSpace(s) == s && len(s) <= max && !strings.ContainsAny(s, "\x00\r\n")
}
func validEventID(id json.RawMessage) bool {
	if len(id) == 0 || len(id) > 200 {
		return false
	}
	var v any
	d := json.NewDecoder(strings.NewReader(string(id)))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return false
	}
	switch x := v.(type) {
	case string:
		return validEventText(x, 190)
	case json.Number:
		return true
	default:
		return false
	}
}

// Keep field structure but never retain arbitrary device values or images.
func redactEvent(v any, depth int) any {
	if depth >= 12 {
		return "[depth limit]"
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any)
		n := 0
		for k, value := range x {
			if n >= 100 {
				out["[truncated]"] = true
				break
			}
			if len(k) > 100 {
				k = "[long key]"
			}
			out[k] = redactEvent(value, depth+1)
			n++
		}
		return out
	case []any:
		out := make([]any, 0)
		for i, value := range x {
			if i >= 20 {
				out = append(out, "[truncated]")
				break
			}
			out = append(out, redactEvent(value, depth+1))
		}
		return out
	case nil:
		return nil
	default:
		return "[redacted]"
	}
}

func registerEventManagement(router gin.IRouter, h *Handler) {
	manage := rbac.RequirePermission(h.permissions, "weighing:camera:manage")
	router.GET("/cameras/:id/events", manage, func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		store, ok := h.service.store.(eventStore)
		if !ok {
			writeResult(c, nil, ErrInvalid)
			return
		}
		binding, err := store.EventBinding(c, id)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		events, err := store.EventDiagnostics(c, id)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		camera, err := h.service.Get(c, id)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		state := "unconfigured"
		if camera.Enabled && binding.Enabled && binding.SerialNumber != "" {
			state = "unknown"
			if binding.LastHeartbeatAt != nil {
				state = "offline"
				if time.Since(*binding.LastHeartbeatAt) >= 0 && time.Since(*binding.LastHeartbeatAt) <= time.Duration(binding.TimeoutSeconds)*time.Second {
					state = "online"
				}
			}
		}
		snapshots := []LicenseSnapshot{}
		if snapshotsStore, ok := h.service.store.(snapshotStore); ok {
			snapshots, err = snapshotsStore.Snapshots(c, id)
			if err != nil {
				writeResult(c, nil, err)
				return
			}
		}
		writeResult(c, gin.H{"binding": binding, "state": state, "events": events, "snapshots": snapshots}, nil)
	})
	router.PUT("/cameras/:id/events", manage, func(c *gin.Context) {
		id, err := pathID(c)
		if err != nil {
			writeResult(c, nil, err)
			return
		}
		var input EventBinding
		if c.ShouldBindJSON(&input) != nil || len(input.SerialNumber) > 200 || (input.SerialNumber != "" && !validEventText(input.SerialNumber, 200)) || (input.Enabled && input.SerialNumber == "") || input.TimeoutSeconds < 10 || input.TimeoutSeconds > 86400 {
			writeResult(c, nil, ErrInvalid)
			return
		}
		store, ok := h.service.store.(eventStore)
		if !ok {
			writeResult(c, nil, ErrInvalid)
			return
		}
		input.CameraID = id
		input.LastHeartbeatAt = nil
		input.DeviceTime = ""
		input.DeviceIP = ""
		err = store.SaveEventBinding(c, input, requestEvent(c))
		writeResult(c, nil, err)
	})
}
