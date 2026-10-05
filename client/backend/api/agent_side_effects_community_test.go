//go:build community

package api

import "testing"

// Agent runs are not compiled into Community. Keeping the assertion at the
// shared test callsite documents that normal deployment must remain local.
func requireNoCommercialAgentRunSideEffects(t *testing.T) { t.Helper() }
