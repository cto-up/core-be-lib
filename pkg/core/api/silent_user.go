package core

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
)

const silentContextKey = "core_silent_user"

// MarkSilent stashes the silent flag on the request context so that the
// generic UserCreatedCallback (which fires for every user creation flow)
// can discriminate silent imports / admin-adds from regular ones without
// adding a new callback type or changing the existing callback signature.
//
// Call this BEFORE invoking userService.CreateUser. The callback receives
// the same gin.Context (typed as context.Context) and reads the flag back
// via IsSilent. Always sets the value (including false) so callers in a
// loop — like the bulk import handler — can flip the flag per row.
func MarkSilent(c *gin.Context, silent bool) {
	c.Set(silentContextKey, silent)
}

// IsSilent reports whether the in-flight user creation was marked silent
// via MarkSilent. Safe to call with any context.Context; returns false for
// non-gin contexts or when the flag is absent.
func IsSilent(ctx context.Context) bool {
	gc, ok := ctx.(*gin.Context)
	if !ok {
		return false
	}
	v, ok := gc.Get(silentContextKey)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// parseBoolFlag converts a CSV cell to a boolean. Accepts y/yes/true/1
// (case-insensitive, surrounding whitespace tolerated). Anything else,
// including empty, is false.
func parseBoolFlag(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes", "true", "1":
		return true
	}
	return false
}
