// Package agentprinter renders and submits the compact SK-820 paper view.
package agentprinter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	_ "embed"
)

type Field struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type Sheet struct {
	Template    string      `json:"template"`
	Width       int         `json:"widthMm"`
	Height      int         `json:"heightMm"`
	Title       string      `json:"title"`
	Marks       []string    `json:"marks"`
	Rows        [][]Field   `json:"rows"`
	Calibration Calibration `json:"calibration,omitempty"`
}
type Calibration struct {
	WidthMM      int `json:"printWidthMm"`
	OffsetXMM    int `json:"offsetXmm"`
	OffsetYMM    int `json:"offsetYmm"`
	FormLengthMM int `json:"formLengthMm"`
}
type Request struct {
	AttemptID int64 `json:"attemptId"`
	Ticket    struct {
		PrintSheet *Sheet `json:"printSheet"`
	} `json:"ticket"`
}

//go:embed render.py
var renderScript string

// Prepare validates the view and strips control characters before rendering.
func Prepare(s *Sheet) ([]byte, error) {
	if s == nil || s.Template != "weighing-compact-v1" || s.Width != 210 || s.Height != 99 || len(s.Rows) != 5 {
		return nil, fmt.Errorf("unsupported print sheet")
	}
	clean := func(text string) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return ' '
			}
			return r
		}, text)
	}
	copySheet := *s
	copySheet.Title = clean(s.Title)
	copySheet.Marks = append([]string{}, s.Marks...)
	for i := range copySheet.Marks {
		copySheet.Marks[i] = clean(copySheet.Marks[i])
	}
	copySheet.Rows = make([][]Field, len(s.Rows))
	for i, row := range s.Rows {
		if len(row) < 1 || len(row) > 3 {
			return nil, fmt.Errorf("unsupported cell count")
		}
		copySheet.Rows[i] = make([]Field, len(row))
		for j, f := range row {
			if len([]rune(f.Label))+len([]rune(f.Value)) > 200 {
				return nil, fmt.Errorf("field too long")
			}
			copySheet.Rows[i][j] = Field{Label: clean(f.Label), Value: clean(f.Value)}
		}
	}
	return json.Marshal(copySheet)
}

// Render rasterizes text and solid table borders together so CJK device font
// metrics cannot misalign the columns. Pillow/font availability is checked onsite.
func Render(ctx context.Context, s *Sheet) ([]byte, error) {
	input, err := Prepare(s)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "python3", "-c", renderScript)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("render: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Submit reserves the attempt durably BEFORE invoking lp. A crash or lp error
// leaves an uncertain attempt reserved; neither restart nor duplicate resubmits it.
func Submit(ctx context.Context, dir, queue string, req Request, data []byte) (string, error) {
	if req.AttemptID < 1 {
		return "", fmt.Errorf("invalid attempt ID")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty print data")
	}
	var err error
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, fmt.Sprintf("attempt-%d.json", req.AttemptID))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return "", fmt.Errorf("attempt already reserved; inspect existing CUPS job")
	}
	if err != nil {
		return "", err
	}
	err = json.NewEncoder(f).Encode(req)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	d, err := os.Open(dir)
	if err != nil {
		return "", err
	}
	err = d.Sync()
	_ = d.Close()
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "lp", "-d", queue, "-o", "raw", "-n", "1", "-t", fmt.Sprintf("weighbridge-attempt-%d", req.AttemptID))
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output))
	_ = os.WriteFile(path+".cups", []byte(fmt.Sprintf("%s\nerror=%v\n", result, err)), 0600)
	if err != nil {
		return result, fmt.Errorf("lp outcome uncertain: %w", err)
	}
	return result, nil
}
