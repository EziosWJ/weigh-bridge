// Package audit owns operation audit events and persistence.
package audit

import "context"

type Metadata struct {
	ActorID int64
	// ActorName is an optional business-facing display name. It lets a system
	// actor (for example the serial QR scanner) attribute a business record to
	// the mobile account that actually initiated it without creating a login.
	ActorName                                                 string
	RequestID, ClientIP, UserAgent, RequestMethod, RequestURL string
}
type Event struct {
	Action, Resource string
	ResourceID       int64
	Summary          string
	Metadata         Metadata
}
type Recorder interface {
	Record(context.Context, Event) error
}
type noop struct{}

func (noop) Record(context.Context, Event) error { return nil }

func Noop() Recorder { return noop{} }
