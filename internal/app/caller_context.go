package app

import (
	"context"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// CallerContext carries transport-derived session and client identity into
// application services without coupling them to MCP or transport protocols.
// CLI-origin calls carry no session hash; MCP sessions carry a process-stable session hash.
type CallerContext struct {
	SessionHash   string
	Client        telemetry.Client
	PriorVerifier func(resolutionID string) bool
}

// VerifyPrior reports whether a prior resolution was issued to this caller session.
func (c CallerContext) VerifyPrior(resolutionID string) bool {
	if c.PriorVerifier != nil {
		return c.PriorVerifier(resolutionID)
	}
	return false
}

type callerContextKey struct{}

// WithCallerContext stores CallerContext in ctx.
func WithCallerContext(ctx context.Context, caller CallerContext) context.Context {
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// CallerFromContext extracts CallerContext from ctx, returning zero value if absent.
func CallerFromContext(ctx context.Context) CallerContext {
	if ctx == nil {
		return CallerContext{}
	}
	caller, ok := ctx.Value(callerContextKey{}).(CallerContext)
	if !ok {
		return CallerContext{}
	}
	return caller
}
