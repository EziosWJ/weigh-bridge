package weighing

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/weighbridge/server/internal/audit"
)

func (s *Service) Originals(ctx context.Context, q PageQuery) (Page[OriginalView], error) {
	return s.store.Originals(ctx, q)
}
func (s *Service) Summary(ctx context.Context, kind string, q PageQuery) (Summary, error) {
	return s.store.Summary(ctx, kind, q)
}
func csvText(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r\n", rune(value[0])) {
		return "'" + value
	}
	return value
}
func csvTime(value time.Time) string { return value.In(beijing).Format("2006-01-02 15:04:05") }
func idText(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}
func encodeCSV(rows [][]string) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.Write([]byte{0xef, 0xbb, 0xbf})
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(rows); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
func (s *Service) Export(ctx context.Context, kind string, q PageQuery, e audit.Event) ([]byte, error) {
	rows := [][]string{}
	count := int64(0)
	switch kind {
	case "originals":
		values, err := s.store.OriginalsAll(ctx, q)
		if err != nil {
			return nil, err
		}
		rows = append(rows, []string{"数据性质", "任务号", "车牌", "运输方向", "首次确认时间（北京）", "二次确认时间（北京）", "原始毛重（吨）", "原始皮重（吨）", "原始净重（吨）", "物料编码", "物料", "规格", "供收方编码", "供收方", "当前单据状态", "首次依据ID", "二次依据ID"})
		for _, v := range values {
			rows = append(rows, []string{"", v.TaskNo, csvText(v.Plate), v.Direction, csvTime(v.FirstConfirmedAt), csvTime(v.SecondConfirmedAt), v.OriginalGross.String(), v.OriginalTare.String(), v.OriginalNet.String(), v.Business.MaterialCode, csvText(v.Business.MaterialName), csvText(v.Business.MaterialSpec), v.Business.PartyCode, csvText(v.Business.PartyName), v.CurrentTicketStatus, idText(v.CurrentFirstRecordID), idText(v.CurrentSecondRecordID)})
		}
		count = int64(len(values))
	case "tickets":
		values, err := s.store.TicketsAll(ctx, q)
		if err != nil {
			return nil, err
		}
		rows = append(rows, []string{"数据性质", "过磅单号", "任务ID", "版本", "关联原单ID", "状态", "出单时间（北京）", "车牌", "方向", "物料编码", "物料", "规格", "供收方编码", "供收方", "原始毛重（吨）", "原始皮重（吨）", "原始净重（吨）", "最终毛重（吨）", "最终皮重（吨）", "最终净重（吨）", "毛重调整差额（吨）", "皮重调整差额（吨）", "毛重来源", "皮重来源", "调整备注", "首次依据ID", "二次依据ID"})
		for _, v := range values {
			rows = append(rows, []string{"", v.TicketNo, strconv.FormatInt(v.TaskID, 10), strconv.FormatInt(v.Revision, 10), idText(v.PreviousTicketID), v.Status, csvTime(v.IssuedAt), csvText(v.Business.Plate), v.Business.Direction, v.Business.MaterialCode, csvText(v.Business.MaterialName), csvText(v.Business.MaterialSpec), v.Business.PartyCode, csvText(v.Business.PartyName), v.OriginalGross.String(), v.OriginalTare.String(), (v.OriginalGross - v.OriginalTare).String(), v.FinalGross.String(), v.FinalTare.String(), (v.FinalGross - v.FinalTare).String(), (v.OriginalGross - v.FinalGross).String(), (v.OriginalTare - v.FinalTare).String(), v.GrossSource, v.TareSource, csvText(v.AdjustmentRemark), strconv.FormatInt(v.FirstRecordID, 10), strconv.FormatInt(v.SecondRecordID, 10)})
		}
		count = int64(len(values))
	case "tasks":
		values, err := s.store.TasksAll(ctx, q)
		if err != nil {
			return nil, err
		}
		rows = append(rows, []string{"数据性质", "任务号", "车牌", "方向", "任务状态", "创建时间（北京）", "物料编码", "物料", "规格", "供收方编码", "供收方", "首次依据ID", "二次依据ID", "司机", "司机电话", "备注"})
		for _, v := range values {
			rows = append(rows, []string{"", v.TaskNo, csvText(v.Plate), v.Direction, v.Status, csvTime(v.CreateTime), v.Business.MaterialCode, csvText(v.Business.MaterialName), csvText(v.Business.MaterialSpec), v.Business.PartyCode, csvText(v.Business.PartyName), idText(v.CurrentFirstRecordID), idText(v.CurrentSecondRecordID), csvText(v.Business.Driver), csvText(v.Business.DriverPhone), csvText(v.Business.Remark)})
		}
		count = int64(len(values))
	case "originals-summary", "tickets-summary":
		source := strings.TrimSuffix(kind, "-summary")
		values, err := s.store.Summary(ctx, source, q)
		if err != nil {
			return nil, err
		}
		rows = append(rows, []string{"数据性质", "方向", "物料ID", "物料", "规格", "供收方ID", "供收方", "有效任务数", "毛重合计（吨）", "皮重合计（吨）", "净重合计（吨）"})
		for _, v := range values.Rows {
			rows = append(rows, []string{"", v.Direction, v.MaterialID, csvText(v.MaterialName), csvText(v.MaterialSpec), v.PartyID, csvText(v.PartyName), strconv.FormatInt(v.TaskCount, 10), v.Gross.String(), v.Tare.String(), v.Net.String()})
		}
		count = int64(len(values.Rows))
	default:
		return nil, ErrInvalid
	}
	data, err := encodeCSV(rows)
	if err != nil {
		return nil, err
	}
	if err := s.store.RecordExport(ctx, kind, q, count, e); err != nil {
		return nil, err
	}
	return data, nil
}
func (s *Service) StartPrint(ctx context.Context, id int64, in PrintInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || (in.Intent != "initial" && in.Intent != "retry" && in.Intent != "reprint") || !maxLength(in.Reason, 2000) {
		return CommandResult{}, ErrInvalid
	}
	// Only a new operation may dispatch a print; replay must never resend paper.
	s.printMu.Lock()
	defer s.printMu.Unlock()
	if op, err := s.store.Operation(ctx, e.Metadata.ActorID, in.RequestID); err == nil && op.Status != "not_found" {
		return s.store.StartPrint(ctx, id, in, e)
	}
	result, err := s.store.StartPrint(ctx, id, in, e)
	if err != nil {
		return result, err
	}
	if s.devices != nil {
		if ticket, lookup := s.store.FindTicket(ctx, id); lookup == nil {
			if task, lookup := s.store.FindTask(ctx, ticket.TaskID); lookup == nil {
				_ = s.devices.Print(task.ScaleID, result.PrintAttemptID, ticket)
			}
		}
	}
	return result, nil
}
func (s *Service) ResolvePrint(ctx context.Context, id int64, in PrintResultInput, e audit.Event) (CommandResult, error) {
	if !validRequestID(in.RequestID) || (in.Status != "printed" && in.Status != "failed" && in.Status != "unknown") || !maxLength(in.Reason, 2000) {
		return CommandResult{}, ErrInvalid
	}
	return s.store.ResolvePrint(ctx, id, in, e)
}
func (s *Service) PrintAttempts(ctx context.Context, id int64) ([]PrintAttempt, error) {
	return s.store.PrintAttempts(ctx, id)
}

func (s *Service) TaskEvents(ctx context.Context, id int64, q PageQuery) (Page[BusinessEvent], error) {
	return s.store.TaskEvents(ctx, id, q)
}
