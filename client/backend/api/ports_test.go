package api

import (
	"dockerpanel/backend/pkg/database"
	"testing"
)

func TestAppendOrMerge(t *testing.T) {
	tests := []struct {
		name     string
		list     []PortRecord
		item     PortRecord
		expected int // expected length of list
	}{
		{
			name: "Merge continuous ports",
			list: []PortRecord{
				{Port: 80, EndPort: 80, Used: true, Type: "Host", Protocol: "TCP"},
			},
			item:     PortRecord{Port: 81, EndPort: 81, Used: true, Type: "Host", Protocol: "TCP"},
			expected: 1,
		},
		{
			name: "Do not merge different status",
			list: []PortRecord{
				{Port: 80, EndPort: 80, Used: true, Type: "Host", Protocol: "TCP"},
			},
			item:     PortRecord{Port: 81, EndPort: 81, Used: false, Type: "", Protocol: "TCP"},
			expected: 2,
		},
		{
			name: "Do not merge non-continuous ports",
			list: []PortRecord{
				{Port: 80, EndPort: 80, Used: true, Type: "Host", Protocol: "TCP"},
			},
			item:     PortRecord{Port: 82, EndPort: 82, Used: true, Type: "Host", Protocol: "TCP"},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendOrMerge(tt.list, tt.item)
			if len(result) != tt.expected {
				t.Errorf("expected length %d, got %d", tt.expected, len(result))
			}
			if tt.expected == 1 && result[0].EndPort != tt.item.EndPort {
				t.Errorf("expected EndPort %d, got %d", tt.item.EndPort, result[0].EndPort)
			}
		})
	}
}

func TestProcessPorts(t *testing.T) {
	usageMap := map[int]PortUsage{
		80: {Used: true, Type: "Host", ServiceName: "http"},
		81: {Used: true, Type: "Host", ServiceName: "http"}, // Should merge with 80
		82: {Used: false},                                   // Gap
		83: {Used: true, Type: "Container", ServiceName: "app"},
	}

	notes := make(map[database.PortNoteKey]string)

	tests := []struct {
		name          string
		opts          FilterOptions
		expectedCount int
	}{
		{
			name: "Filter All",
			opts: FilterOptions{
				Start: 80, End: 83,
				Protocol: "all", Used: "all", ExactPort: -1,
			},
			expectedCount: 3, // 80-81 (merged), 82, 83
		},
		{
			name: "Filter Used",
			opts: FilterOptions{
				Start: 80, End: 83,
				Protocol: "all", Used: "true", ExactPort: -1,
			},
			expectedCount: 2, // 80-81 (merged), 83
		},
		{
			name: "Filter Type Host",
			opts: FilterOptions{
				Start: 80, End: 83,
				Protocol: "all", TypeValues: map[string]bool{"host": true}, Used: "all", ExactPort: -1,
			},
			expectedCount: 1, // 80-81 (merged)
		},
		{
			name: "Filter Type Container (Exclude Unused)",
			opts: FilterOptions{
				Start: 80, End: 83,
				Protocol: "all", TypeValues: map[string]bool{"container": true}, Used: "all", ExactPort: -1,
			},
			expectedCount: 1, // 83 only. 82 is unused so it should be excluded even if its type is empty/unknown
		},
		{
			name: "Filter Exact Port",
			opts: FilterOptions{
				Start: 80, End: 83,
				Protocol: "all", Used: "all", ExactPort: 82,
			},
			expectedCount: 1, // 82
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := processPorts(tt.opts.Start, tt.opts.End, "TCP", usageMap, tt.opts, notes)
			if len(results) != tt.expectedCount {
				t.Errorf("expected %d records, got %d", tt.expectedCount, len(results))
			}
		})
	}
}

func TestAutoPortAllocationAllowed(t *testing.T) {
	if autoPortAllocationAllowed("ports-page", false) {
		t.Fatal("all allocation calls should be blocked when disabled")
	}
	if autoPortAllocationAllowed("compose:demo", false) {
		t.Fatal("automated allocation should be blocked when disabled")
	}
	if !autoPortAllocationAllowed("compose:demo", true) {
		t.Fatal("automated allocation should be allowed when enabled")
	}
}
