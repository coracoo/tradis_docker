package database

import (
	"testing"
	"time"
)

func TestNowStampShapeAndParses(t *testing.T) {
	stamp := NowStamp()
	// NowStamp 必须是带 +08:00 偏移的 RFC3339，go-sqlite3 才会保留偏移读回正确瞬间。
	if _, err := time.Parse(time.RFC3339, stamp); err != nil {
		t.Fatalf("NowStamp not RFC3339: %q (%v)", stamp, err)
	}
	parsed, ok := ParseStoredTime(stamp)
	if !ok {
		t.Fatalf("NowStamp output not parseable: %q", stamp)
	}
	if parsed.Location() != chinaLocation {
		t.Fatalf("parsed NowStamp not in China location: %v", parsed.Location())
	}
}

func TestFormatStampNormalizesAnyLocationToChina(t *testing.T) {
	// UTC instant 2026-08-05T15:00:00Z → China 2026-08-05T23:00:00+08:00
	utc := time.Date(2026, 8, 5, 15, 0, 0, 0, time.UTC)
	if got := FormatStamp(utc); got != "2026-08-05T23:00:00+08:00" {
		t.Fatalf("FormatStamp(utc) = %q, want 2026-08-05T23:00:00+08:00", got)
	}
	// Already-China instant is unchanged.
	cn := time.Date(2026, 8, 5, 23, 0, 0, 0, chinaLocation)
	if got := FormatStamp(cn); got != "2026-08-05T23:00:00+08:00" {
		t.Fatalf("FormatStamp(cn) = %q, want 2026-08-05T23:00:00+08:00", got)
	}
	if got := FormatStamp(time.Time{}); got != "" {
		t.Fatalf("FormatStamp(zero) = %q, want empty", got)
	}
}

func TestParseStoredTimeHandlesRFC3339AndNaive(t *testing.T) {
	// RFC3339 in UTC → returned in China location with the right wall clock.
	rfc, ok := ParseStoredTime("2026-08-05T07:00:00Z")
	if !ok {
		t.Fatalf("RFC3339 parse failed")
	}
	if rfc.Location() != chinaLocation {
		t.Fatalf("RFC3339 parsed location = %v, want China", rfc.Location())
	}
	if got := rfc.Format(sqliteStampLayout); got != "2026-08-05 15:00:00" {
		t.Fatalf("RFC3339 China wall clock = %q, want 2026-08-05 15:00:00", got)
	}

	// Naive string → interpreted as China time.
	naive, ok := ParseStoredTime("2026-08-05 23:12:28")
	if !ok {
		t.Fatalf("naive parse failed")
	}
	if naive.Location() != chinaLocation {
		t.Fatalf("naive parsed location = %v, want China", naive.Location())
	}

	// Empty / garbage.
	if _, ok := ParseStoredTime(""); ok {
		t.Fatalf("empty string should not parse")
	}
	if _, ok := ParseStoredTime("not a timestamp"); ok {
		t.Fatalf("garbage should not parse")
	}
}
