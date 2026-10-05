package api

import (
	"dockerpanel/backend/pkg/database"
	"strings"
)

// FilterOptions defines the criteria for filtering ports
type FilterOptions struct {
	Start      int
	End        int
	Protocol   string          // "tcp", "udp", "all"
	TypeValues map[string]bool // optional multi-value type filter
	Used       string          // "true", "false", "all"
	ExactPort  int             // -1 if not searching for a specific port
}

// Helper to merge continuous items
func appendOrMerge(list []PortRecord, item PortRecord) []PortRecord {
	if len(list) == 0 {
		return append(list, item)
	}
	last := &list[len(list)-1]

	// Determine if we can merge
	// Same status (Used), same Type, same Service, same Protocol, same Note
	// And ports must be contiguous: item.Port == last.EndPort + 1
	canMerge := (last.Used == item.Used) &&
		(last.Type == item.Type) &&
		(last.Service == item.Service) &&
		(last.Protocol == item.Protocol) &&
		(last.Note == item.Note) &&
		(item.Port == last.EndPort+1)

	if canMerge {
		last.EndPort = item.EndPort
		return list
	}
	return append(list, item)
}

// createRecordHelper creates a PortRecord from usage info
func createRecordHelper(p int, proto string, usage PortUsage, notes map[database.PortNoteKey]string) PortRecord {
	t := usage.Type
	if !usage.Used {
		t = ""
	}

	keyType := t
	note := ""
	key := database.PortNoteKey{Port: p, Type: keyType, Protocol: strings.ToUpper(proto)}
	if n, ok := notes[key]; ok {
		note = n
	}

	return PortRecord{
		Port:     p,
		EndPort:  p,
		Type:     t,
		Protocol: strings.ToUpper(proto),
		Used:     usage.Used,
		Note:     note,
		Service:  usage.ServiceName,
	}
}

// processPorts iterates over a range and applies filters, returning merged records
func processPorts(start, end int, proto string, usageMap map[int]PortUsage, opts FilterOptions, notes map[database.PortNoteKey]string) []PortRecord {
	var records []PortRecord
	for p := start; p <= end; p++ {
		usage := usageMap[p]

		// Used filter
		if opts.Used == "true" && !usage.Used {
			continue
		}
		if opts.Used == "false" && usage.Used {
			continue
		}

		// Type filter
		if len(opts.TypeValues) > 0 {
			if !usage.Used {
				continue
			}
			if !opts.TypeValues[strings.ToLower(usage.Type)] {
				continue
			}
		}

		if opts.ExactPort != -1 && p != opts.ExactPort {
			continue
		}

		item := createRecordHelper(p, proto, usage, notes)
		records = appendOrMerge(records, item)
	}
	return records
}
