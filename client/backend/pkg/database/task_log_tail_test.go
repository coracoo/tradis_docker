package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskLogTailOrderingScopeAndLimits(t *testing.T) {
	setupNotificationTestDB(t)
	require.NoError(t, UpsertTask("tail", "compose_deploy", "running"))
	tx, err := db.Begin()
	require.NoError(t, err)
	defer tx.Rollback()
	for seq := 1; seq <= 2010; seq++ {
		_, err = tx.Exec(`INSERT INTO task_logs (task_id, environment_id, seq, message) VALUES (?, ?, ?, ?)`, "tail", LocalEnvironmentID, seq, "progress")
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	for _, tc := range []struct{ limit, count, first int }{{12, 12, 1999}, {0, 500, 1511}, {3000, 2000, 11}} {
		logs, err := GetTaskLogTail("tail", tc.limit)
		require.NoError(t, err)
		require.Len(t, logs, tc.count)
		require.Equal(t, int64(tc.first), logs[0].Seq)
		require.Equal(t, int64(2010), logs[len(logs)-1].Seq)
		for i := 1; i < len(logs); i++ {
			require.Greater(t, logs[i].Seq, logs[i-1].Seq)
		}
	}
	logs, err := GetTaskLogTail("", 12)
	require.NoError(t, err)
	require.Empty(t, logs)
	logs, err = GetTaskLogsAfter("tail", 0, 12)
	require.NoError(t, err)
	require.Len(t, logs, 12)
	require.Equal(t, int64(1), logs[0].Seq, "SSE must still page from the beginning")
	logs, err = GetTaskLogsAfter("tail", 2005, 12)
	require.NoError(t, err)
	require.Len(t, logs, 5)
	require.Equal(t, int64(2006), logs[0].Seq)
}
