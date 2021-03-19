package proxy

import (
	"context"
)

// A generalize representation of a pipe
// A pipe represents a stream of proxy traffic for a given connection, either "up" (from client to upstream", or "down"
// (from upstream back to client)
type Pipe interface {
	Run(ctx context.Context) error
}
