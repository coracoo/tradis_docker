package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/database"
	"github.com/gin-gonic/gin"
)

func TestScheduledTasksWithoutEnvironmentParameterUseLocal(t *testing.T) {
	if current := database.GetDB(); current != nil {
		_ = current.Close()
	}
	if err := database.InitDB(filepath.Join(t.TempDir(), "scheduled-tasks.db")); err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		if current := database.GetDB(); current != nil {
			_ = current.Close()
		}
	})
	local := database.ScheduledJob{Name: "本机任务", TaskType: "image_update_check", CronExpr: "@daily", Enabled: true}
	if err := database.CreateScheduledJob(&local); err != nil {
		t.Fatalf("CreateScheduledJob() error = %v", err)
	}

	router := gin.New()
	RegisterScheduledTaskRoutes(router.Group("/api"))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/scheduled-tasks", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /scheduled-tasks = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if body := response.Body.String(); !strings.Contains(body, "本机任务") {
		t.Fatalf("scheduled tasks response omitted local data: %s", body)
	}
}
