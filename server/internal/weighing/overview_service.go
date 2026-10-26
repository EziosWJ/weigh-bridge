package weighing

import (
	"context"
	"time"
)

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	return s.store.Overview(ctx, time.Now().UTC())
}
