package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dockerpanel/backend/pkg/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskEventsDrainEveryLogPageBeforeTerminalResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTaskTestDB(t)

	const taskID = "task-stream-paged"
	require.NoError(t, database.UpsertTask(taskID, "test", "running"))
	for seq := int64(1); seq <= 501; seq++ {
		require.NoError(t, database.AppendTaskLogWithSeq(
			taskID,
			seq,
			time.Now(),
			"info",
			fmt.Sprintf("log-%03d", seq),
		))
	}
	require.NoError(t, database.FinishTask(taskID, "success", nil, ""))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/events", nil)
	streamDatabaseTaskEvents(ctx, taskID)

	body := recorder.Body.String()
	lastLog := strings.Index(body, "log-501")
	result := strings.Index(body, `"type":"result"`)
	require.NotEqual(t, -1, lastLog)
	require.NotEqual(t, -1, result)
	require.Less(t, lastLog, result)
}
