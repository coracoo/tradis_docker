package api

import (
	"dockerpanel/backend/pkg/database"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const remoteEnvironmentHeader = "X-TRADIS-Environment"

// All explicit targets must agree, including repeated query/header values.
func requestEnvironmentScope(c *gin.Context) (string, bool) {
	values := append([]string{}, c.Request.Header.Values(remoteEnvironmentHeader)...)
	query := c.Request.URL.Query()
	values = append(values, query["environmentId"]...)
	values = append(values, query["environment_id"]...)
	target := ""
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if target != "" && target != value {
			respondErrorWithCode(c, http.StatusBadRequest, "environment_conflict", "请求中的目标设备不一致", nil)
			return "", false
		}
		target = value
	}
	if target == "" {
		target = database.LocalEnvironmentID
	}
	return target, true
}
