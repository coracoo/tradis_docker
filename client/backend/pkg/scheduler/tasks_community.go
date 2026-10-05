//go:build community

package scheduler

import (
	"context"
	"fmt"
	"time"

	"dockerpanel/backend/pkg/database"
)

func scheduledTaskTimeout(database.ScheduledJob) time.Duration {
	return 30 * time.Minute
}

func dispatchEditionJob(_ context.Context, job database.ScheduledJob, _ string, _ *int64) error {
	return fmt.Errorf("未知任务类型: %s", job.TaskType)
}
