//go:build community

package settings

const SupportsAppStoreContentSource = true

// Community builds contain no official origin to redact. Keep the shared
// diagnostics and backup call sites stable without embedding that origin.
func RedactAppStoreURL(text string) string { return text }
