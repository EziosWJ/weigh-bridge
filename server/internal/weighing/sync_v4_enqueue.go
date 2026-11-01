package weighing

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// enqueueTicketChangeV4 writes one digest-protocol change together with the
// business action. The digest is computed once here and, together with the
// payload, is never regenerated when the change is retried.
func enqueueTicketChangeV4(tx *gorm.DB, ticket Ticket, previous *Ticket, kind, actor string) error {
	change := syncChangeV4{
		SchemaVersion: SyncSchemaVersionV4,
		DigestVersion: SyncDigestVersion2,
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
	snapshot, err := buildTicketSnapshot(ticket, previous)
	if err != nil {
		return err
	}
	if previous != nil {
		change.PreviousSiteCode = previous.SiteCode
	}
	change.Ticket = snapshot
	digest, err := computeChangeDigestV4(change)
	if err != nil {
		return err
	}
	change.Digest = digest
	payload, err := json.Marshal(change)
	if err != nil {
		return err
	}
	previousID := ticket.PreviousTicketID
	if previous != nil {
		value := previous.ID
		previousID = &value
	}
	now := time.Now().UTC()
	return tx.Create(&ticketSyncChangeV4{
		TicketID:         ticket.ID,
		PreviousTicketID: previousID,
		Digest:           digest,
		DigestVersion:    SyncDigestVersion2,
		SchemaVersion:    SyncSchemaVersionV4,
		SiteCode:         ticket.SiteCode,
		TicketNo:         ticket.TicketNo,
		ChangeType:       kind,
		Payload:          string(payload),
		Status:           SyncChangePending,
		NextAttemptAt:    &now,
	}).Error
}
