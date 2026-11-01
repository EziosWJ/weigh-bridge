package weighing

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"
)

// Digest rules version 2, as published in the shared factory/cloud release
// document. The canonical byte string is the UTF-8 prefix
// "wb-change-digest\n" followed, in fixed order, by fields encoded as
// name + ":" + UTF-8 byte length + ":" + value + "\n". Nullable fields encode
// as name + ":null\n". schemaVersion and digest do not participate.
const syncDigestPrefix = "wb-change-digest\n"

func computeChangeDigestV4(c syncChangeV4) (string, error) {
	if c.SchemaVersion != SyncSchemaVersionV4 || c.DigestVersion != SyncDigestVersion2 {
		return "", ErrInvalid
	}
	if !siteCodePattern.MatchString(c.SiteCode) || c.TicketNo == "" || len(c.TicketNo) > 200 {
		return "", ErrInvalid
	}
	var b strings.Builder
	b.WriteString(syncDigestPrefix)
	writeDigestField(&b, "rulesVersion", strconv.Itoa(SyncDigestVersion2))
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
	if err := writeTicketDigestFields(&b, c.Ticket); err != nil {
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

func writeTicketDigestFields(b *strings.Builder, t *syncTicket) error {
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
	if t.NetAdjustmentMode != "amount" && t.NetAdjustmentMode != "percent" {
		return ErrInvalid
	}
	writeDigestField(b, "netAdjustmentMode", t.NetAdjustmentMode)
	if t.NetAdjustmentRateBP == nil {
		writeDigestNull(b, "netAdjustmentRateBp")
	} else {
		if *t.NetAdjustmentRateBP < -10000 || *t.NetAdjustmentRateBP > 10000 {
			return ErrInvalid
		}
		writeDigestField(b, "netAdjustmentRateBp", strconv.Itoa(*t.NetAdjustmentRateBP))
	}
	writeDigestField(b, "netAdjustmentCategory", t.NetAdjustmentCategory)
	writeDigestField(b, "netAdjustmentCategoryName", t.NetAdjustmentCategoryName)
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

func writeDigestField(b *strings.Builder, name, value string) {
	b.WriteString(name)
	b.WriteByte(':')
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('\n')
}

func writeDigestNull(b *strings.Builder, name string) {
	b.WriteString(name)
	b.WriteString(":null\n")
}

// canonicalDigestWeight parses a decimal tonne string with at most three
// decimals into integer milli-tonnes and re-formats it as sign + d.ddd.
func canonicalDigestWeight(value string, signed bool) (string, error) {
	if value == "" {
		return "", ErrInvalid
	}
	negative := value[0] == '-'
	if negative {
		value = value[1:]
	} else if value[0] == '+' {
		return "", ErrInvalid
	}
	if value == "" {
		return "", ErrInvalid
	}
	whole, fraction := value, ""
	if index := strings.IndexByte(value, '.'); index >= 0 {
		whole, fraction = value[:index], value[index+1:]
		if fraction == "" || len(fraction) > 3 {
			return "", ErrInvalid
		}
	}
	if whole == "" {
		return "", ErrInvalid
	}
	for _, ch := range whole + fraction {
		if ch < '0' || ch > '9' {
			return "", ErrInvalid
		}
	}
	if negative && !signed {
		return "", ErrInvalid
	}
	wholeValue, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return "", ErrInvalid
	}
	for len(fraction) < 3 {
		fraction += "0"
	}
	fractionValue, _ := strconv.ParseInt(fraction, 10, 64)
	if wholeValue > (math.MaxInt64-fractionValue)/1000 {
		return "", ErrInvalid
	}
	milli := wholeValue*1000 + fractionValue
	if negative {
		milli = -milli
	}
	sign := ""
	if milli < 0 {
		sign = "-"
		milli = -milli
	}
	return sign + strconv.FormatInt(milli/1000, 10) + "." + padMilli(milli%1000), nil
}

func padMilli(value int64) string {
	text := strconv.FormatInt(value, 10)
	for len(text) < 3 {
		text = "0" + text
	}
	return text
}

func canonicalDigestTime(value time.Time) (string, error) {
	if value.IsZero() {
		return "", ErrInvalid
	}
	return value.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z"), nil
}
