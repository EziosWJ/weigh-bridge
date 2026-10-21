package agentproto

import "errors"

// ErrKicked ends a client process normally, unlike a recoverable network failure.
var ErrKicked = errors.New("Agent 已被后台踢出")
