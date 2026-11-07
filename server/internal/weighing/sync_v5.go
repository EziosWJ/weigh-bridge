package weighing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Digest-protocol (schemaVersion 5) constants for multi-item net adjustment.
// The wire shape reuses syncChangeV4 with a per-item array; legacy v3/v4
// changes keep their own tables, payloads and digests untouched.
const (
	SyncSchemaVersionV5 = 5
	SyncDigestVersion3  = 3
)

// syncNetAdjustmentItemV5 is one detail item on the wire. Amount is the signed
// resolved tonne string; RateBP is null for amount items.
type syncNetAdjustmentItemV5 struct {
	Mode         string `json:"mode"`
	Amount       string `json:"amount"`
	RateBP       *int   `json:"rateBp"`
	Category     string `json:"category"`
	CategoryName string `json:"categoryName"`
	Remark       string `json:"remark"`
}

func (t *syncTicket) buildNetAdjustmentsV5(items []NetAdjustmentItem) []syncNetAdjustmentItemV5 {
	list := make([]syncNetAdjustmentItemV5, 0, len(items))
	for _, item := range items {
		amount := WeightDifference(item.AmountMilli)
		list = append(list, syncNetAdjustmentItemV5{Mode: item.Mode, Amount: amount.String(), RateBP: item.RateBP, Category: item.Category, CategoryName: item.CategoryName, Remark: item.Remark})
	}
	return list
}

// enqueueTicketChangeV5 writes one digest-protocol v5 change. The digest is
// computed once here and never regenerated when retried.
func enqueueTicketChangeV5(tx *gorm.DB, ticket Ticket, previous *Ticket, kind, actor string) error {
	change := syncChangeV4{
		SchemaVersion: SyncSchemaVersionV5,
		DigestVersion: SyncDigestVersion3,
		SiteCode:      ticket.SiteCode,
		TicketNo:      ticket.TicketNo,
		Type:          kind,
	}
	if kind == "void" {
		void, err := buildVoidSnapshot(ticket, actor)
		if err != nil {
			return err
		}
		change.Void = void
		if ticket.PreviousTicketID != nil {
			prior, err := findOne[Ticket](tx, *ticket.PreviousTicketID)
			if err != nil {
				return err
			}
			previous = &prior
		}
	}
	snapshot, err := buildTicketSnapshotV5(ticket, previous)
	if err != nil {
		return err
	}
	if previous != nil {
		change.PreviousSiteCode = previous.SiteCode
	}
	change.Ticket = snapshot.ticket
	digest, err := computeChangeDigestV5(change, snapshot.items)
	if err != nil {
		return err
	}
	change.Digest = digest
	payload, err := json.Marshal(struct {
		syncChangeV4
		Ticket *syncTicketV5 `json:"ticket,omitempty"`
	}{syncChangeV4: change, Ticket: snapshot.wire})
	if err != nil {
		return err
	}
	previousID := ticket.PreviousTicketID
	if previous != nil {
		value := previous.ID
		previousID = &value
	}
	now := time.Now().UTC()
	return tx.Create(&ticketSyncChangeV5{
		TicketID:         ticket.ID,
		PreviousTicketID: previousID,
		Digest:           digest,
		DigestVersion:    SyncDigestVersion3,
		SchemaVersion:    SyncSchemaVersionV5,
		SiteCode:         ticket.SiteCode,
		TicketNo:         ticket.TicketNo,
		ChangeType:       kind,
		Payload:          string(payload),
		Status:           SyncChangePending,
		NextAttemptAt:    &now,
	}).Error
}

// syncTicketV5 is the v5 wire ticket: the v4 shape with netAdjustments and
// without the legacy single-item fields that described the whole ticket.
type syncTicketV5 struct {
	syncTicket
	NetAdjustments []syncNetAdjustmentItemV5 `json:"netAdjustments"`
}

type v5Snapshot struct {
	ticket *syncTicket
	wire   *syncTicketV5
	items  []syncNetAdjustmentItemV5
}

func buildTicketSnapshotV5(ticket Ticket, previous *Ticket) (v5Snapshot, error) {
	if len(ticket.Basis) != 2 {
		return v5Snapshot{}, errSyncMissingBasis
	}
	b := ticket.Business
	items := ticket.NetAdjustmentItems
	if items == nil {
		items = []NetAdjustmentItem{}
	}
	base := &syncTicket{TicketNo: ticket.TicketNo, Title: ticket.TicketTitle, Plate: b.Plate, Direction: b.Direction, MaterialCode: b.MaterialCode, MaterialName: b.MaterialName, MaterialSpec: b.MaterialSpec, PartyCode: b.PartyCode, PartyName: b.PartyName, FirstConfirmedAt: ticket.Basis[0].ConfirmedAt, SecondConfirmedAt: ticket.Basis[1].ConfirmedAt, IssuedAt: ticket.IssuedAt, IssuerName: ticket.IssuerName, OriginalGross: ticket.OriginalGross.String(), OriginalTare: ticket.OriginalTare.String(), FinalGross: ticket.FinalGross.String(), FinalTare: ticket.FinalTare.String(), FinalNet: finalNet(ticket.FinalGross, ticket.FinalTare, ticket.NetAdjustment).String(), NetAdjustment: ticket.NetAdjustment.String(), GrossSource: ticket.GrossSource, TareSource: ticket.TareSource, AdjustmentRemark: ticket.AdjustmentRemark, AdjustedAt: ticket.AdjustedAt, Driver: b.Driver, DriverPhone: b.DriverPhone, ContactName: b.ContactName, ContactPhone: b.ContactPhone}
	if ticket.AdjustedBy != nil {
		base.AdjustedBy = &ticket.IssuerName
	}
	if previous != nil {
		base.PreviousTicketNo = &previous.TicketNo
		gross, tare := previous.FinalGross.String(), previous.FinalTare.String()
		net := finalNet(previous.FinalGross, previous.FinalTare, previous.NetAdjustment).String()
		adjustment := previous.NetAdjustment.String()
		base.PreviousFinalGross = &gross
		base.PreviousFinalTare = &tare
		base.PreviousFinalNet = &net
		base.PreviousNetAdjustment = &adjustment
	}
	wire := &syncTicketV5{syncTicket: *base, NetAdjustments: (&syncTicket{}).buildNetAdjustmentsV5(items)}
	// The digest uses the shared v4 field writer for everything except the
	// single-item fields, which v5 replaced by the per-item array below.
	return v5Snapshot{ticket: base, wire: wire, items: wire.NetAdjustments}, nil
}

var errSyncMissingBasis = errorString("missing ticket basis")

type errorString string

func (e errorString) Error() string { return string(e) }

// computeChangeDigestV5 follows digest rules version 3: identical to v2
// except that the four single-item net-adjustment fields are replaced by the
// ordered netAdjustments array (count, then per-item fields in fixed order).
func computeChangeDigestV5(c syncChangeV4, items []syncNetAdjustmentItemV5) (string, error) {
	if c.SchemaVersion != SyncSchemaVersionV5 || c.DigestVersion != SyncDigestVersion3 {
		return "", ErrInvalid
	}
	if !siteCodePattern.MatchString(c.SiteCode) || c.TicketNo == "" || len(c.TicketNo) > 200 {
		return "", ErrInvalid
	}
	var b strings.Builder
	b.WriteString(syncDigestPrefix)
	writeDigestField(&b, "rulesVersion", strconv.Itoa(SyncDigestVersion3))
	writeDigestField(&b, "siteCode", c.SiteCode)
	writeDigestField(&b, "ticketNo", c.TicketNo)
	writeDigestField(&b, "changeType", c.Type)
	switch c.Type {
	case "issue":
		if c.Ticket == nil || c.Void != nil || c.PreviousSiteCode != "" || c.Ticket.PreviousTicketNo != nil || c.Ticket.PreviousFinalGross != nil || c.Ticket.PreviousFinalTare != nil || c.Ticket.PreviousFinalNet != nil || c.Ticket.PreviousNetAdjustment != nil {
			return "", ErrInvalid
		}
		writeDigestNull(&b, "previousSiteCode")
		writeDigestNull(&b, "previousTicketNo")
	case "reopen":
		if c.Ticket == nil || c.Void != nil || !siteCodePattern.MatchString(c.PreviousSiteCode) || c.Ticket.PreviousTicketNo == nil || *c.Ticket.PreviousTicketNo == "" {
			return "", ErrInvalid
		}
		writeDigestField(&b, "previousSiteCode", c.PreviousSiteCode)
		writeDigestField(&b, "previousTicketNo", *c.Ticket.PreviousTicketNo)
	case "void":
		if c.Void == nil || c.Ticket == nil {
			return "", ErrInvalid
		}
		if c.Ticket.PreviousTicketNo != nil {
			if !siteCodePattern.MatchString(c.PreviousSiteCode) || *c.Ticket.PreviousTicketNo == "" {
				return "", ErrInvalid
			}
			writeDigestField(&b, "previousSiteCode", c.PreviousSiteCode)
			writeDigestField(&b, "previousTicketNo", *c.Ticket.PreviousTicketNo)
		} else {
			if c.PreviousSiteCode != "" {
				return "", ErrInvalid
			}
			writeDigestNull(&b, "previousSiteCode")
			writeDigestNull(&b, "previousTicketNo")
		}
	default:
		return "", ErrInvalid
	}
	if c.Ticket.TicketNo != c.TicketNo {
		return "", ErrInvalid
	}
	if err := writeTicketDigestFieldsV5(&b, c.Ticket, items); err != nil {
		return "", err
	}
	if c.Type == "void" {
		if c.Void.Reason == "" || c.Void.Actor == "" {
			return "", ErrInvalid
		}
		writeDigestField(&b, "voidReason", c.Void.Reason)
		writeDigestField(&b, "voidActor", c.Void.Actor)
		at, err := canonicalDigestTime(c.Void.At)
		if err != nil {
			return "", err
		}
		writeDigestField(&b, "voidAt", at)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:]), nil
}

func writeTicketDigestFieldsV5(b *strings.Builder, t *syncTicket, items []syncNetAdjustmentItemV5) error {
	if t.TicketNo == "" || t.Title == "" || t.Plate == "" || t.MaterialCode == "" || t.MaterialName == "" || t.PartyCode == "" || t.PartyName == "" || t.IssuerName == "" || (t.Direction != "inbound" && t.Direction != "outbound") {
		return ErrInvalid
	}
	for _, value := range []struct{ name, value string }{
		{"title", t.Title}, {"plate", t.Plate}, {"direction", t.Direction},
		{"materialCode", t.MaterialCode}, {"materialName", t.MaterialName}, {"materialSpec", t.MaterialSpec},
		{"partyCode", t.PartyCode}, {"partyName", t.PartyName},
	} {
		writeDigestField(b, value.name, value.value)
	}
	for _, value := range []struct {
		name string
		time time.Time
	}{{"firstConfirmedAt", t.FirstConfirmedAt}, {"secondConfirmedAt", t.SecondConfirmedAt}, {"issuedAt", t.IssuedAt}} {
		canonical, err := canonicalDigestTime(value.time)
		if err != nil {
			return err
		}
		writeDigestField(b, value.name, canonical)
	}
	writeDigestField(b, "issuerName", t.IssuerName)
	for _, value := range []struct {
		name   string
		value  string
		signed bool
	}{
		{"originalGross", t.OriginalGross, false}, {"originalTare", t.OriginalTare, false},
		{"finalGross", t.FinalGross, false}, {"finalTare", t.FinalTare, false},
		{"finalNet", t.FinalNet, false}, {"netAdjustment", t.NetAdjustment, true},
	} {
		canonical, err := canonicalDigestWeight(value.value, value.signed)
		if err != nil {
			return err
		}
		writeDigestField(b, value.name, canonical)
	}
	// v5: ordered item array instead of the v4 single-item fields.
	writeDigestField(b, "netAdjustmentsCount", strconv.Itoa(len(items)))
	for index, item := range items {
		prefix := "netAdjustments." + strconv.Itoa(index) + "."
		if item.Mode != "amount" && item.Mode != "percent" {
			return ErrInvalid
		}
		writeDigestField(b, prefix+"mode", item.Mode)
		canonical, err := canonicalDigestWeight(item.Amount, true)
		if err != nil {
			return err
		}
		writeDigestField(b, prefix+"amount", canonical)
		if item.RateBP == nil {
			if item.Mode != "amount" {
				return ErrInvalid
			}
			writeDigestNull(b, prefix+"rateBp")
		} else {
			if item.Mode != "percent" || *item.RateBP < -10000 || *item.RateBP > 10000 {
				return ErrInvalid
			}
			writeDigestField(b, prefix+"rateBp", strconv.Itoa(*item.RateBP))
		}
		writeDigestField(b, prefix+"category", item.Category)
		writeDigestField(b, prefix+"categoryName", item.CategoryName)
		writeDigestField(b, prefix+"remark", item.Remark)
	}
	for _, value := range []struct {
		name   string
		value  *string
		signed bool
	}{
		{"previousFinalGross", t.PreviousFinalGross, false}, {"previousFinalTare", t.PreviousFinalTare, false},
		{"previousFinalNet", t.PreviousFinalNet, false}, {"previousNetAdjustment", t.PreviousNetAdjustment, true},
	} {
		if value.value == nil {
			writeDigestNull(b, value.name)
			continue
		}
		canonical, err := canonicalDigestWeight(*value.value, value.signed)
		if err != nil {
			return err
		}
		writeDigestField(b, value.name, canonical)
	}
	writeDigestField(b, "grossSource", t.GrossSource)
	writeDigestField(b, "tareSource", t.TareSource)
	writeDigestField(b, "adjustmentRemark", t.AdjustmentRemark)
	if t.AdjustedBy == nil {
		writeDigestNull(b, "adjustedBy")
	} else {
		writeDigestField(b, "adjustedBy", *t.AdjustedBy)
	}
	if t.AdjustedAt == nil {
		writeDigestNull(b, "adjustedAt")
	} else {
		canonical, err := canonicalDigestTime(*t.AdjustedAt)
		if err != nil {
			return err
		}
		writeDigestField(b, "adjustedAt", canonical)
	}
	writeDigestField(b, "driver", t.Driver)
	writeDigestField(b, "driverPhone", t.DriverPhone)
	writeDigestField(b, "contactName", t.ContactName)
	writeDigestField(b, "contactPhone", t.ContactPhone)
	return nil
}
