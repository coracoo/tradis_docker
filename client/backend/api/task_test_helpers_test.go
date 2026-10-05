package api

import (
	"path/filepath"
	"testing"

	"dockerpanel/backend/pkg/database"
	"github.com/stretchr/testify/require"
)

func initTaskTestDB(t *testing.T) {
	t.Helper()
	_ = database.Close()
	require.NoError(t, database.InitDB(filepath.Join(t.TempDir(), "data.db")))
	t.Cleanup(func() { _ = database.Close() })
}
