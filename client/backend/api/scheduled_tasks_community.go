//go:build community

package api

import (
	"fmt"
	"net/http"
	"strings"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/scheduler"
)

// RegisterCommercialSchedulerCallbacks is intentionally empty in the
// community build. Keeping the symbol makes the bootstrap contract explicit
// without bringing protection execution into the public artifact.
func RegisterCommercialSchedulerCallbacks() {}

func RegisterSchedulerCallbacks() {
	RegisterCommunitySchedulerCallbacks()
}

func isScheduledTaskTypeAvailable(taskType string) bool {
	switch taskType {
	case scheduler.TaskContainerStart, scheduler.TaskContainerStop, scheduler.TaskContainerRestart,
		scheduler.TaskImageUpdateCheck, scheduler.TaskBuildCachePrune:
		return true
	default:
		return false
	}
}

func validateScheduledTaskEditionInput(scheduledTaskInput, bool) (string, bool) {
	return "", true
}

func validateScheduledTaskEditionUpdate(existing database.ScheduledJob, requested scheduledTaskInput) (int, error) {
	if !isScheduledTaskTypeAvailable(existing.TaskType) {
		return http.StatusBadRequest, fmt.Errorf("当前版本不支持该定时任务类型")
	}
	return 0, nil
}

func scheduledTaskTarget(_ database.ScheduledJob, requested string) string {
	return strings.TrimSpace(requested)
}

func applyScheduledTaskEditionUpdate(database.ScheduledJob) error {
	return nil
}

func afterScheduledTaskDelete(database.ScheduledJob) error {
	return nil
}
