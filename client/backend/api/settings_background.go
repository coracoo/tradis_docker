package api

import (
	"context"
	"dockerpanel/backend/pkg/background"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/settings"
	"dockerpanel/backend/pkg/system"
	"sync"
)

const (
	settingsContainerDiscoveryTaskKey = "settings.container-discovery"
	settingsVolumeBackupTaskKey       = "settings.volume-backup"
)

var settingsBackground = struct {
	sync.RWMutex
	runner    *background.Runner
	discovery func(context.Context, settings.Settings)
	backup    func(context.Context, settings.Settings)
}{
	discovery: func(ctx context.Context, _ settings.Settings) {
		system.ProcessContainerDiscoveryContext(ctx)
	},
	backup: system.EnsureVolumeBackupContainerContext,
}

// SetSettingsBackgroundRunner configures the process-owned runner used by
// settings maintenance. Passing nil clears the configuration for test cleanup.
func SetSettingsBackgroundRunner(runner *background.Runner) {
	settingsBackground.Lock()
	settingsBackground.runner = runner
	settingsBackground.Unlock()
}

// ScheduleSettingsMaintenance starts the settings-dependent maintenance tasks
// asynchronously. The runner coalesces repeated saves for each fixed task key.
func ScheduleSettingsMaintenance(saved settings.Settings) {
	snapshot := copySettingsForMaintenance(saved)

	settingsBackground.RLock()
	runner := settingsBackground.runner
	discovery := settingsBackground.discovery
	backup := settingsBackground.backup
	settingsBackground.RUnlock()
	if runner == nil {
		logging.Error("settings background runner is not configured")
		return
	}

	runner.Submit(settingsContainerDiscoveryTaskKey, func(ctx context.Context) error {
		discovery(ctx, snapshot)
		return nil
	})
	runner.Submit(settingsVolumeBackupTaskKey, func(ctx context.Context) error {
		backup(ctx, snapshot)
		return nil
	})
}

func copySettingsForMaintenance(saved settings.Settings) settings.Settings {
	snapshot := saved
	snapshot.VolumeBackupVolumes = append([]string(nil), saved.VolumeBackupVolumes...)
	snapshot.NotificationEnabledCategories = append([]string(nil), saved.NotificationEnabledCategories...)
	return snapshot
}
