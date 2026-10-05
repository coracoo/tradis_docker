package api

import (
	"dockerpanel/backend/pkg/database"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func isFinishedTaskStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "error", "failed", "completed", "waiting_input", "cancelled", "canceled":
		return true
	default:
		return false
	}
}

// Local-only operations must never silently reinterpret a remote target.
func localEnvironmentScope(c *gin.Context) (string, bool) {
	environmentID, ok := requestEnvironmentScope(c)
	if !ok {
		return "", false
	}
	if environmentID != database.LocalEnvironmentID {
		respondError(c, 400, "当前版本仅支持本机环境", nil)
		return "", false
	}
	return environmentID, true
}

func streamDatabaseTaskEvents(c *gin.Context, taskID string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		respondError(c, 400, "任务ID不能为空", nil)
		return
	}
	environmentID, ok := localEnvironmentScope(c)
	if !ok {
		return
	}
	streamDatabaseTaskEventsInEnvironment(c, environmentID, taskID)
}

func streamDatabaseTaskEventsInEnvironment(c *gin.Context, environmentID, taskID string) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		respondError(c, 400, "任务ID不能为空", nil)
		return
	}

	setSSEHeaders(c)
	ctx := c.Request.Context()
	afterSeq := sseNextIDFromLastEventID(c) - 1
	if afterSeq < 0 {
		afterSeq = 0
	}

	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	writeLine := func(seq int64, payload any) {
		sseWriteJSONEvent(c, seq, "message", payload)
		c.Writer.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		logs, err := database.GetTaskLogsAfterInEnvironment(environmentID, taskID, afterSeq, 500)
		if err != nil {
			writeLine(afterSeq+1, gin.H{"type": "error", "message": "读取任务日志失败: " + err.Error(), "time": time.Now().Format(time.RFC3339)})
			return
		}
		for _, r := range logs {
			writeLine(r.Seq, gin.H{"type": strings.TrimSpace(r.Type), "message": r.Message, "time": r.Time})
			afterSeq = r.Seq
		}
		// A full page can still have persisted logs behind it. Drain those pages
		// before emitting the terminal result, otherwise reconnecting clients can
		// permanently miss the tail of a completed task.
		if len(logs) == 500 {
			continue
		}

		t, terr := database.GetTaskInEnvironment(environmentID, taskID)
		if terr == nil && isFinishedTaskStatus(t.Status) {
			// Logs can be committed between the page read and the terminal-state
			// read. Recheck after observing the terminal state before closing SSE.
			tail, err := database.GetTaskLogsAfterInEnvironment(environmentID, taskID, afterSeq, 1)
			if err != nil {
				return
			}
			if len(tail) != 0 {
				continue
			}
			writeLine(afterSeq+1, gin.H{"type": "result", "status": strings.ToLower(strings.TrimSpace(t.Status)), "taskId": taskID, "error": t.Error})
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			_, _ = fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		case <-time.After(800 * time.Millisecond):
		}
	}
}
