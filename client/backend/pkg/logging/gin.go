package logging

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

func GinMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}
		args := []any{
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(startedAt).Milliseconds(),
		}

		switch status := c.Writer.Status(); {
		case status >= http.StatusInternalServerError:
			Error("http request", args...)
		case status >= http.StatusBadRequest:
			Warn("http request", args...)
		default:
			Debug("http request", args...)
		}
	}
}

func GinRecovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		args := []any{"error", fmt.Sprint(recovered)}
		if Enabled(slog.LevelDebug) {
			args = append(args, "stack", string(debug.Stack()))
		}
		Error("panic recovered", args...)
		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
