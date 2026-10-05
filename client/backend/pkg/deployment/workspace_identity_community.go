//go:build community

package deployment

// WorkspaceIdentity is kept only for decoding shared deployment plans. The
// Community edition never creates or executes AI workspace candidates.
type WorkspaceIdentity struct{}
