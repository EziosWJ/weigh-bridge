package weighing

import (
	"context"
	"encoding/base64"
	"sort"
	"strconv"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
	"github.com/EziosWJ/weighbridge/server/internal/camera"
	"github.com/gin-gonic/gin"
)

func (h *Handler) confirmPlateRecognition(c *gin.Context) {
	var input PlateConfirmationInput
	if c.ShouldBindJSON(&input) != nil || input.ScaleCode == "" {
		writeResult(c, nil, ErrInvalid)
		return
	}
	store, ok := h.service.store.(interface {
		ConfirmPlate(context.Context, PlateConfirmationInput, audit.Event) (int64, error)
	})
	if !ok {
		writeResult(c, nil, ErrConditions)
		return
	}
	id, err := store.ConfirmPlate(c, input, requestEvent(c))
	writeResult(c, map[string]int64{"confirmationId": id}, err)
}

type plateCandidateResponse struct {
	Items      []camera.PlateCandidate `json:"items"`
	NextCursor string                  `json:"nextCursor"`
	ServerTime time.Time               `json:"serverTime"`
}

func plateCursor(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(id, 10)))
}
func parsePlateCursor(value string) (int64, error) {
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, ErrInvalid
	}
	id, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil || id < 0 {
		return 0, ErrInvalid
	}
	return id, nil
}

// Advance no further than the first group omitted by the page limit.
func paginateCandidates(groups []camera.PlateCandidate, after, watermark int64, initial bool) ([]camera.PlateCandidate, int64) {
	if initial {
		if len(groups) > 5 {
			groups = groups[:5]
		}
		return groups, watermark
	}
	type pending struct {
		group camera.PlateCandidate
		first int64
	}
	var remaining []pending
	for _, group := range groups {
		var first int64
		for _, event := range group.SourceEvents {
			if event.ID > after && (first == 0 || event.ID < first) {
				first = event.ID
			}
		}
		if first != 0 {
			remaining = append(remaining, pending{group, first})
		}
	}
	sort.Slice(remaining, func(i, j int) bool { return remaining[i].first < remaining[j].first })
	cursor := watermark
	if len(remaining) > 5 {
		cursor = remaining[5].first - 1
		remaining = remaining[:5]
	}
	items := make([]camera.PlateCandidate, 0, len(remaining))
	for _, v := range remaining {
		items = append(items, v.group)
	}
	return items, cursor
}

func (h *Handler) plateCandidates(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	status, err := h.service.LocalStatus(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	scale := c.Query("scale")
	if !status.Initialized || scale == "" || scale != status.ScaleCode || h.service.camera == nil {
		writeResult(c, nil, ErrForbidden)
		return
	}
	scales, err := h.service.Scales(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	enabled, match := 0, false
	for _, item := range scales {
		if item.Enabled == 1 {
			enabled++
			match = item.Code == scale
		}
	}
	if enabled != 1 || !match {
		writeResult(c, nil, ErrForbidden)
		return
	}
	after := int64(0)
	initial := c.Query("after") == ""
	if !initial {
		after, err = parsePlateCursor(c.Query("after"))
		if err != nil {
			writeResult(c, nil, err)
			return
		}
	}
	events, watermark, now, err := h.service.camera.CandidateWindow(c)
	if err != nil {
		writeResult(c, nil, err)
		return
	}
	if after > watermark {
		writeResult(c, nil, ErrInvalid)
		return
	}
	valid := events[:0]
	for _, event := range events {
		if businessPlate.MatchString(event.Plate) {
			valid = append(valid, event)
		}
	}
	items, next := paginateCandidates(camera.AggregatePlateCandidates(valid, now), after, watermark, initial)
	writeResult(c, plateCandidateResponse{Items: items, NextCursor: plateCursor(next), ServerTime: now}, nil)
}
