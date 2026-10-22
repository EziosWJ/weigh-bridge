package agentprinter

import (
	"errors"
	"fmt"
	"math"
)

const printDPI = 180.0

// Geometry describes the actual dot-quantized image, before and after offsets.
type Geometry struct {
	WidthMM         float64 `json:"widthMm"`
	HeightMM        float64 `json:"heightMm"`
	FeedMM          float64 `json:"feedMm"`
	EffectiveFormMM float64 `json:"effectiveFormMm"`
}

func (c Calibration) scale() float64 {
	if c.Scale == 0 {
		return 1
	}
	return c.Scale
}

func (c Calibration) dimensions() (width, height, feed, form int) {
	width = int(math.Round(math.Round(float64(c.WidthMM)*printDPI/25.4) * c.scale()))
	height = int(math.Round(600 * c.scale()))
	feed = (height + int(math.Round(float64(c.OffsetYMM)*printDPI/25.4)) + 23) / 24 * 24
	form = int(math.Round(c.FormLengthMM * 360 / 25.4))
	return
}

func (c Calibration) Geometry() Geometry {
	w, h, feed, form := c.dimensions()
	return Geometry{float64(w) * 25.4 / printDPI, float64(h) * 25.4 / printDPI, float64(feed) * 25.4 / printDPI, float64(form) * 25.4 / 360}
}

func (c Calibration) Validate() error {
	for _, v := range []float64{c.FormLengthMM, c.Scale} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("打印参数必须为有限数值")
		}
	}
	if c.WidthMM < 100 || c.WidthMM > 210 || c.scale() < 0.5 || c.scale() > 1.5 || c.OffsetXMM < 0 || c.OffsetXMM > 30 || c.OffsetYMM < 0 || c.OffsetYMM > 30 || c.FormLengthMM < 85 || c.FormLengthMM > 300 {
		return errors.New("打印尺寸、缩放或偏移超出允许范围")
	}
	w, _, feed, form := c.dimensions()
	if w+int(math.Round(float64(c.OffsetXMM)*printDPI/25.4)) > int(math.Round(210*printDPI/25.4)) || feed*2 > form {
		return errors.New("缩放后的内容或分带走纸超出纸宽／表长，请减小缩放或偏移")
	}
	return nil
}

// CalibrationSheet shares business rendering but never loads a business ticket.
func CalibrationSheet(c Config) *Sheet {
	cal := c.Calibration()
	g := cal.Geometry()
	return &Sheet{Template: "weighing-compact-v1", Width: 210, Height: 99,
		Title: "打印校准测试样张", Marks: []string{"测试／校准 · 非业务单据 · 标尺 mm"},
		Calibration: cal, CalibrationPattern: true, Rows: [][]Field{
			{{Label: "基准宽度／缩放", Value: fmt.Sprintf("%d mm / %g 倍", c.PrintWidthMM, cal.Scale)}},
			{{Label: "实际内容宽高", Value: fmt.Sprintf("%.2f × %.2f mm", g.WidthMM, g.HeightMM)}},
			{{Label: "右移／下移", Value: fmt.Sprintf("%d / %d mm", c.OffsetXMM, c.OffsetYMM)}},
			{{Label: "表长／指令值", Value: fmt.Sprintf("%g / %.3f mm", c.FormLengthMM, g.EffectiveFormMM)}},
			{{Label: "队列", Value: c.Queue}},
		}}
}
