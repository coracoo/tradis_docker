package api

import (
	"encoding/json"
	"net/http"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/registryprobe"
	"github.com/gin-gonic/gin"
)

var registryMirrorProbeCheck = registryprobe.Check
var registryMirrorProbeSlots = make(chan struct{}, 2)

// checkRegistryMirror is a shared local-only diagnostic, not a Docker pull or configuration write.
func checkRegistryMirror(c *gin.Context) {
	environmentID, valid := requestEnvironmentScope(c)
	if !valid {
		return
	}
	if environmentID != database.LocalEnvironmentID {
		respondErrorWithCode(c, http.StatusNotImplemented, "mirror_probe_remote_unsupported", "远程 Agent 暂不支持镜像源检测，请在目标 NAS 本机检测", nil)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var input struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&input); err != nil {
		respondErrorWithCode(c, http.StatusBadRequest, "invalid_mirror_url", "镜像源请求格式无效", nil)
		return
	}
	base, err := registryprobe.NormalizeURL(input.URL)
	if err != nil {
		respondErrorWithCode(c, http.StatusBadRequest, "invalid_mirror_url", err.Error(), nil)
		return
	}
	select {
	case registryMirrorProbeSlots <- struct{}{}:
		defer func() { <-registryMirrorProbeSlots }()
	default:
		c.Header("Retry-After", "6")
		respondErrorWithCode(c, http.StatusTooManyRequests, "mirror_probe_busy", "镜像源检测繁忙，请稍后重试", nil)
		return
	}
	c.JSON(http.StatusOK, registryMirrorProbeCheck(c.Request.Context(), base))
}
