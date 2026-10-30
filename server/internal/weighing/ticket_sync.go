package weighing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

type syncTicket struct {
	TicketNo                  string     `json:"ticketNo"`
	PreviousTicketNo          *string    `json:"previousTicketNo"`
	Title                     string     `json:"title"`
	Plate                     string     `json:"plate"`
	Direction                 string     `json:"direction"`
	MaterialCode              string     `json:"materialCode"`
	MaterialName              string     `json:"materialName"`
	MaterialSpec              string     `json:"materialSpec"`
	PartyCode                 string     `json:"partyCode"`
	PartyName                 string     `json:"partyName"`
	FirstConfirmedAt          time.Time  `json:"firstConfirmedAt"`
	SecondConfirmedAt         time.Time  `json:"secondConfirmedAt"`
	IssuedAt                  time.Time  `json:"issuedAt"`
	IssuerName                string     `json:"issuerName"`
	OriginalGross             string     `json:"originalGross"`
	OriginalTare              string     `json:"originalTare"`
	FinalGross                string     `json:"finalGross"`
	FinalTare                 string     `json:"finalTare"`
	FinalNet                  string     `json:"finalNet"`
	NetAdjustment             string     `json:"netAdjustment"`
	NetAdjustmentMode         string     `json:"netAdjustmentMode"`
	NetAdjustmentRateBP       *int       `json:"netAdjustmentRateBp"`
	NetAdjustmentCategory     string     `json:"netAdjustmentCategory"`
	NetAdjustmentCategoryName string     `json:"netAdjustmentCategoryName"`
	PreviousFinalGross        *string    `json:"previousFinalGross"`
	PreviousFinalTare         *string    `json:"previousFinalTare"`
	PreviousFinalNet          *string    `json:"previousFinalNet"`
	PreviousNetAdjustment     *string    `json:"previousNetAdjustment"`
	GrossSource               string     `json:"grossSource"`
	TareSource                string     `json:"tareSource"`
	AdjustmentRemark          string     `json:"adjustmentRemark"`
	AdjustedBy                *string    `json:"adjustedBy"`
	AdjustedAt                *time.Time `json:"adjustedAt"`
	Driver                    string     `json:"driver"`
	DriverPhone               string     `json:"driverPhone"`
	ContactName               string     `json:"contactName"`
	ContactPhone              string     `json:"contactPhone"`
}
type syncVoid struct {
	Reason string    `json:"reason"`
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
}
type syncChange struct {
	SchemaVersion    int         `json:"schemaVersion"`
	ChangeID         string      `json:"changeId"`
	SiteCode         string      `json:"siteCode"`
	PreviousSiteCode string      `json:"previousSiteCode,omitempty"`
	TicketNo         string      `json:"ticketNo"`
	Type             string      `json:"type"`
	Ticket           *syncTicket `json:"ticket,omitempty"`
	Void             *syncVoid   `json:"void,omitempty"`
}
type ticketSyncChange struct {
	ID               int64 `gorm:"primaryKey"`
	TicketID         int64
	PreviousTicketID *int64
	ChangeID         string
	TicketNo         string
	ChangeType       string
	Payload          string
	Status           string
	Reason           string
}

func (ticketSyncChange) TableName() string { return "wb_ticket_sync_change" }

// enqueueTicketChange runs in the same transaction as the business action.
func enqueueTicketChange(tx *gorm.DB, ticket Ticket, previous *Ticket, kind, actor string) error {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	c := syncChange{SchemaVersion: 3, ChangeID: hex.EncodeToString(id[:]), SiteCode: ticket.SiteCode, TicketNo: ticket.TicketNo, Type: kind}
	if kind == "void" {
		if ticket.VoidedAt == nil {
			return errors.New("missing void time")
		}
		c.Void = &syncVoid{Reason: ticket.VoidReason, Actor: actor, At: *ticket.VoidedAt}
	} else {
		if len(ticket.Basis) != 2 {
			return errors.New("missing ticket basis")
		}
		b := ticket.Business
		t := &syncTicket{TicketNo: ticket.TicketNo, Title: ticket.TicketTitle, Plate: b.Plate, Direction: b.Direction, MaterialCode: b.MaterialCode, MaterialName: b.MaterialName, MaterialSpec: b.MaterialSpec, PartyCode: b.PartyCode, PartyName: b.PartyName, FirstConfirmedAt: ticket.Basis[0].ConfirmedAt, SecondConfirmedAt: ticket.Basis[1].ConfirmedAt, IssuedAt: ticket.IssuedAt, IssuerName: ticket.IssuerName, OriginalGross: ticket.OriginalGross.String(), OriginalTare: ticket.OriginalTare.String(), FinalGross: ticket.FinalGross.String(), FinalTare: ticket.FinalTare.String(), FinalNet: finalNet(ticket.FinalGross, ticket.FinalTare, ticket.NetAdjustment).String(), NetAdjustment: ticket.NetAdjustment.String(), NetAdjustmentMode: ticket.NetAdjustmentMode, NetAdjustmentRateBP: ticket.NetAdjustmentRateBP, NetAdjustmentCategory: ticket.NetAdjustmentCategory, NetAdjustmentCategoryName: ticket.NetAdjustmentCategoryName, GrossSource: ticket.GrossSource, TareSource: ticket.TareSource, AdjustmentRemark: ticket.AdjustmentRemark, AdjustedAt: ticket.AdjustedAt, Driver: b.Driver, DriverPhone: b.DriverPhone, ContactName: b.ContactName, ContactPhone: b.ContactPhone}
		if ticket.AdjustedBy != nil {
			t.AdjustedBy = &ticket.IssuerName
		}
		if previous != nil {
			c.PreviousSiteCode = previous.SiteCode
			t.PreviousTicketNo = &previous.TicketNo
			gross, tare := previous.FinalGross.String(), previous.FinalTare.String()
			net := finalNet(previous.FinalGross, previous.FinalTare, previous.NetAdjustment).String()
			adjustment := previous.NetAdjustment.String()
			t.PreviousFinalGross = &gross
			t.PreviousFinalTare = &tare
			t.PreviousFinalNet = &net
			t.PreviousNetAdjustment = &adjustment
		}
		c.Ticket = t
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return tx.Create(&ticketSyncChange{TicketID: ticket.ID, PreviousTicketID: ticket.PreviousTicketID, ChangeID: c.ChangeID, TicketNo: c.TicketNo, ChangeType: kind, Payload: string(payload), Status: "pending"}).Error
}

func postTicketChange(ctx context.Context, address string, change ticketSyncChange) error {
	u, err := url.Parse(address)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("云端地址无效")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/weighbridge/sync/changes"
	u.RawPath = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), strings.NewReader(change.Payload))
	if err != nil {
		return errors.New("云端请求无效")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("连接云端失败或结果未知")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return errors.New("云端拒绝厂区编码：编码不存在或已停用")
	}
	if resp.StatusCode == 409 {
		return errors.New("云端拒绝变化：单号或变化冲突，或前序状态不满足")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("云端 HTTP 响应失败 (%d)", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return errors.New("云端响应读取失败，结果未知")
	}
	var envelope struct {
		Code int `json:"code"`
		Data struct {
			ChangeID string `json:"changeId"`
			TicketNo string `json:"ticketNo"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || envelope.Code != 200 || envelope.Data.ChangeID != change.ChangeID || envelope.Data.TicketNo != change.TicketNo {
		return errors.New("云端确认无效，结果未知")
	}
	return nil
}

// RunTicketSync attempts each eligible change once per pass; failures do not block other tickets.
func (s *Service) RunTicketSync(ctx context.Context) error {
	if !s.masterSyncEnabled {
		return nil
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return ErrForbidden
	}
	s.ticketSyncMu.Lock()
	defer s.ticketSyncMu.Unlock()
	var changes []ticketSyncChange
	if err := repo.db.WithContext(ctx).Raw(`SELECT c.* FROM wb_ticket_sync_change c WHERE c.status <> 'synced' AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_change p WHERE p.ticket_id = c.ticket_id AND p.id < c.id AND p.status <> 'synced') AND (c.previous_ticket_id IS NULL OR (EXISTS (SELECT 1 FROM wb_ticket_sync_change v WHERE v.ticket_id=c.previous_ticket_id AND v.change_type='void' AND v.status='synced') AND NOT EXISTS (SELECT 1 FROM wb_ticket_sync_change p WHERE p.ticket_id=c.previous_ticket_id AND p.status <> 'synced'))) ORDER BY c.last_attempt_at ASC NULLS FIRST, c.id LIMIT 100`).Scan(&changes).Error; err != nil {
		return err
	}
	for _, change := range changes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := postTicketChange(ctx, s.masterSyncAddress, change)
		reason := ""
		status := "synced"
		if err != nil {
			reason = err.Error()
			status = "failed"
		}
		dbErr := repo.db.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
			values := map[string]any{"status": status, "reason": reason, "last_attempt_at": time.Now().UTC()}
			if err == nil {
				values["synced_at"] = time.Now().UTC()
			}
			if e := tx.Model(&ticketSyncChange{}).Where("id=? AND status <> 'synced'", change.ID).Updates(values).Error; e != nil {
				return e
			}
			return tx.Table("wb_master_sync_log").Create(map[string]any{"trigger_type": "automatic", "sync_type": change.ChangeType, "ticket_no": change.TicketNo, "success": err == nil, "reason": reason}).Error
		})
		if dbErr != nil {
			return dbErr
		}
	}
	return nil
}
