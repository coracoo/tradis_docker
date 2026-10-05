//go:build community

package api

import "dockerpanel/backend/pkg/settings"

type diagnosticSettings struct {
	AdvancedMode              bool `json:"advanced_mode"`
	AutoPortAllocationEnabled bool `json:"auto_port_allocation_enabled"`
	VolumeBackupEnabled       bool `json:"volume_backup_enabled"`
	VolumeBackupRemoteSet     bool `json:"volume_backup_remote_configured"`
	NotificationCategoryCount int  `json:"notification_category_count"`
}

func populateEditionDiagnosticSettings(*diagnosticSettings, settings.Settings) {}
