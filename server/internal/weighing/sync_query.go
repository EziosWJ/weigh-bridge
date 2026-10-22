package weighing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

var syncImageName = regexp.MustCompile(`^[0-9]+_[a-f0-9]{32}_(front|rear)\.(png|jpg)$`)

func (r *Repository) SiteFreshness(ctx context.Context, q PageQuery) ([]SiteFreshness, error) {
	rows := []SiteFreshness{}
	db := r.db.WithContext(ctx).Table("wb_site s").Select("s.id AS site_id,COALESCE(n.last_success,'') AS last_success").Joins("LEFT JOIN wb_sync_state n ON n.site_id=s.id")
	err := siteFilter(db, q, "s.id").Order("s.id").Find(&rows).Error
	return rows, err
}
func (s *Service) decorateTicketImages(ticket *TicketView) {
	if s.node.Role != "central" {
		return
	}
	complete := true
	data, _ := json.Marshal(ticket.Basis)
	var value any
	_ = json.Unmarshal(data, &value)
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		case string:
			name := filepath.Base(x)
			if syncImageName.MatchString(name) {
				if _, err := os.Stat(filepath.Join(s.imageDir, name)); err != nil {
					complete = false
				}
			}
		}
	}
	walk(value)
	ticket.ImagesComplete = &complete
}
