//go:build community

package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCommunitySettingsDoNotExposeCommercialConfigurationFields(t *testing.T) {
	payload, err := json.Marshal(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(payload)
	for _, field := range []string{
		`"aiApiKey"`,
		`"aiAgentPrompt"`,
		`"githubAppSearchToken"`,
		`"tutorialCacheLimitMB"`,
	} {
		if strings.Contains(serialized, field) {
			t.Fatalf("community settings exposed commercial field %s: %s", field, serialized)
		}
	}
	if !strings.Contains(serialized, `"aiEnabled"`) || !strings.Contains(serialized, `"aiModel"`) {
		t.Fatalf("community settings omitted free AI fields: %s", serialized)
	}
	if !strings.Contains(serialized, `"tutorialRSSURL"`) {
		t.Fatalf("community settings omitted public tutorial source: %s", serialized)
	}
}
