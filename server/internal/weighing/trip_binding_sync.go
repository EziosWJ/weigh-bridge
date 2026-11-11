package weighing

import (
	"context"
	"net/http"
	"strings"

	"github.com/EziosWJ/weighbridge/server/internal/tripsync"
)

// linkTripBinding is called after the cloud confirms an issue change. A link
// failure never rolls back the local ticket or the sync receipt; the binding
// stays in ticketed and is retried by retryPendingTripLinks.
func (s *Service) linkTripBinding(ctx context.Context, ticketID int64, ticketNo string) {
	if !s.masterSyncEnabled || strings.TrimSpace(s.masterSyncAddress) == "" || ticketID <= 0 || ticketNo == "" {
		return
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return
	}
	var binding TripBinding
	if err := repo.db.WithContext(ctx).Where("ticket_id=? AND status='ticketed'", ticketID).Take(&binding).Error; err != nil {
		return
	}
	if err := tripsync.LinkTicket(ctx, s.masterSyncAddress, binding.SiteCode, binding.TripNo, ticketNo); err != nil {
		if status, ok := tripsync.StatusOf(err); ok && (status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusNotFound) {
			_ = repo.markTripBindingConflict(context.WithoutCancel(ctx), binding.ID)
		}
		return
	}
	_ = repo.markTripBindingLinked(context.WithoutCancel(ctx), binding.ID, ticketNo)
}

func (s *Service) retryPendingTripLinks(ctx context.Context) {
	if !s.masterSyncEnabled || strings.TrimSpace(s.masterSyncAddress) == "" {
		return
	}
	repo, ok := s.store.(*Repository)
	if !ok {
		return
	}
	rows, err := repo.pendingTripLinks(ctx, 50)
	if err != nil {
		return
	}
	for _, row := range rows {
		if ctx.Err() != nil {
			return
		}
		if err := tripsync.LinkTicket(ctx, s.masterSyncAddress, row.SiteCode, row.TripNo, row.IssuedTicketNo); err != nil {
			if status, ok := tripsync.StatusOf(err); ok && (status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusNotFound) {
				_ = repo.markTripBindingConflict(context.WithoutCancel(ctx), row.ID)
			}
			continue
		}
		_ = repo.markTripBindingLinked(context.WithoutCancel(ctx), row.ID, row.IssuedTicketNo)
	}
}
