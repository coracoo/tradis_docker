//go:build community

package settings

import "strings"

func initEditionSettingsDefaults(insert func(string, string) error) error {
	if err := insert("tutorial_rss_url", ""); err != nil {
		return err
	}
	return freeAISettingsDefaults(insert)
}

func loadEditionSettings(s *Settings, value func(string) string, parseInt func(string, int) int, parseBool func(string, bool) bool) {
	s.TutorialRSSURL = strings.TrimSpace(value("tutorial_rss_url"))
	loadFreeAISettings(s, value, parseInt, parseBool)
}

func updateEditionSettings(s *Settings, update func(string, string) error) error {
	if err := update("tutorial_rss_url", strings.TrimSpace(s.TutorialRSSURL)); err != nil {
		return err
	}
	return updateFreeAISettings(s, update)
}

func isEditionSensitiveSettingKey(string) bool { return false }

func defaultNotificationEnabledCategories() []string {
	return []string{"deploy_task", "volume_backup_task", "system"}
}
