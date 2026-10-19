// Package agentproto defines version 1 of the shared device communication contract.
package agentproto

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Weight int64

func (v Weight) String() string {
	n := int64(v)
	sign := ""
	digits := strconv.FormatInt(n, 10)
	if n < 0 {
		sign = "-"
		digits = digits[1:]
	}
	for len(digits) < 4 {
		digits = "0" + digits
	}
	whole, fraction := digits[:len(digits)-3], strings.TrimRight(digits[len(digits)-3:], "0")
	if fraction != "" {
		whole += "." + fraction
	}
	return sign + whole
}
func (v Weight) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func (v *Weight) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return fmt.Errorf("weight must be a tonne decimal string")
	}
	parts := strings.Split(text, ".")
	if len(parts) > 2 || parts[0] == "" || (len(parts[0]) > 1 && parts[0][0] == '0') {
		return fmt.Errorf("invalid weight")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) < 1 || len(fraction) > 3 {
			return fmt.Errorf("invalid precision")
		}
	}
	for _, ch := range parts[0] + fraction {
		if ch < '0' || ch > '9' {
			return fmt.Errorf("invalid weight")
		}
	}
	for len(fraction) < 3 {
		fraction += "0"
	}
	n, err := strconv.ParseInt(parts[0]+fraction, 10, 64)
	if err != nil {
		return err
	}
	*v = Weight(n)
	return nil
}

type Message struct {
	Version    int             `json:"version"`
	Type       string          `json:"type"`
	MessageID  string          `json:"messageId"`
	RequestID  string          `json:"requestId,omitempty"`
	OccurredAt time.Time       `json:"occurredAt"`
	Payload    json.RawMessage `json:"payload"`
}

func New(kind, id, request string, payload any) Message {
	data, _ := json.Marshal(payload)
	return Message{1, kind, id, request, time.Now().UTC(), data}
}

type DeviceState struct {
	Weight   Weight `json:"weight"`
	Unit     string `json:"unit"`
	Stable   bool   `json:"stable"`
	DeviceOK bool   `json:"deviceOk"`
	Error    string `json:"error,omitempty"`
}
type Plate struct {
	Candidate string    `json:"candidate"`
	Failed    bool      `json:"failed"`
	At        time.Time `json:"at"`
}
type Image struct {
	ID     string `json:"id"`
	View   string `json:"view"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}
type Camera struct {
	Images []Image `json:"images"`
}
type Capture struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	DeviceState
	CapturedAt time.Time `json:"capturedAt"`
	Images     []Image   `json:"images"`
	Plate      Plate     `json:"plate"`
}
type PrintRequest struct {
	AttemptID int64 `json:"attemptId"`
	Ticket    any   `json:"ticket"`
}
type PrintResult struct {
	AttemptID int64  `json:"attemptId"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
}
