package notification

import "sync"

// Change is a hint to reload persisted state, never a copy of message content.
type Change struct {
	Type string `json:"type"`
	ID   int64  `json:"id,omitempty"`
}
type subscription struct {
	userID  int64
	changes chan Change
}
type Hub struct {
	mu          sync.Mutex
	closed      bool
	subscribers map[*subscription]struct{}
}

func NewHub() *Hub { return &Hub{subscribers: map[*subscription]struct{}{}} }
func (h *Hub) subscribe(userID int64) (*subscription, func()) {
	s := &subscription{userID: userID, changes: make(chan Change, 32)}
	h.mu.Lock()
	if h.closed {
		close(s.changes)
	} else {
		h.subscribers[s] = struct{}{}
	}
	h.mu.Unlock()
	return s, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subscribers[s]; ok {
			delete(h.subscribers, s)
			close(s.changes)
		}
	}
}
func (h *Hub) send(change Change, match func(int64) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subscribers {
		if !match(s.userID) {
			continue
		}
		select {
		case s.changes <- change:
		default:
			delete(h.subscribers, s)
			close(s.changes)
		}
	}
}
func (h *Hub) Notify(userIDs []int64, change Change) {
	targets := make(map[int64]bool, len(userIDs))
	for _, id := range userIDs {
		targets[id] = true
	}
	h.send(change, func(id int64) bool { return id > 0 && targets[id] })
}
func (h *Hub) Announce(scope string) {
	h.send(Change{Type: "announcement"}, func(id int64) bool { return (scope == ScopePublic && id == 0) || (scope == ScopeInternal && id > 0) })
}
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subscribers {
		close(s.changes)
		delete(h.subscribers, s)
	}
}
func (s *Service) Close() { s.hub.Close() }
