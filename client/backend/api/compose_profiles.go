package api

import (
	"fmt"
	"regexp"
	"strings"
)

var composeProfileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// normalizeComposeProfiles accepts the API's legacy profile shapes while
// keeping values safe to pass to docker compose --profile. It is deliberately
// independent from the AI deployment package because manual Compose uses the
// same command construction.
func normalizeComposeProfiles(value any) []string {
	var candidates []string
	switch typed := value.(type) {
	case []string:
		candidates = typed
	case []any:
		candidates = make([]string, 0, len(typed))
		for _, item := range typed {
			candidates = append(candidates, fmt.Sprint(item))
		}
	case string:
		candidates = strings.Split(typed, ",")
	}

	profiles := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		profile := strings.TrimSpace(candidate)
		if !composeProfileNamePattern.MatchString(profile) {
			continue
		}
		if _, exists := seen[profile]; exists {
			continue
		}
		seen[profile] = struct{}{}
		profiles = append(profiles, profile)
	}
	return profiles
}
