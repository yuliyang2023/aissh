package agent

import "context"

// Provider is the AI backend used by the SSH workspace. Implementations run on
// the local machine; they never need to be installed on the SSH target.
type Provider interface {
	Name() string
	Configured() bool
	Stream(context.Context, Request, Callbacks) (Turn, error)
	Action(context.Context, Request) (Turn, error)
}
