package api

import "testing"

func TestNormalizeKVRouteKeyAllowsPersistentUIPreferences(t *testing.T) {
	for _, key := range []string{
		kvOverviewStatOrderKey,
		kvResourceColumnWidths,
		kvResourceTableColumns,
		kvNavigationShowHidden,
		kvOnboardingTourKey,
	} {
		t.Run(key, func(t *testing.T) {
			normalized, ok := normalizeKVRouteKey(key)
			if !ok {
				t.Fatalf("expected %q to be an allowed KV route key", key)
			}
			if normalized != key {
				t.Fatalf("expected normalized key %q, got %q", key, normalized)
			}
		})
	}
}
