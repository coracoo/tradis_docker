package api

import (
	"context"
	"crypto/rand"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"
	"github.com/gin-gonic/gin"
)

const (
	volumeBrowseImage = "filebrowser/filebrowser:v2.59.0"
	volumeBrowseTTL   = 2 * time.Minute
)

type volumeBrowseSession struct {
	ID          string
	VolumeName  string
	ContainerID string
	TargetHost  string
	ReadOnly    bool
	LastSeen    time.Time
}

var (
	volumeBrowseMu       sync.Mutex
	volumeBrowseSessions = map[string]*volumeBrowseSession{}
	volumeBrowseOnce     sync.Once
)

func startVolumeBrowse(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	volumeName := strings.TrimSpace(c.Param("name"))
	if volumeName == "" {
		respondError(c, http.StatusBadRequest, "invalid volume", nil)
		return
	}

	cli, err := docker.NewDockerClient()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "docker client init failed", err)
		return
	}
	defer cli.Close()
	ctx := c.Request.Context()

	if _, err := cli.VolumeInspect(ctx, volumeName); err != nil {
		respondError(c, http.StatusNotFound, "volume not found", err)
		return
	}

	readOnly := isVolumeInUse(ctx, cli, volumeName)

	if err := ensureImage(ctx, cli, volumeBrowseImage); err != nil {
		respondError(c, http.StatusInternalServerError, "pull image failed", err)
		return
	}

	sid := newSessionID()
	containerName := "tradis-volume-browser-" + sid

	baseURL := "/api/volumes/browse/" + sid + "/fb/"
	env := []string{
		"FB_NOAUTH=true",
		"FB_BASEURL=" + baseURL,
		"UID=0",
		"GID=0",
	}

	exposed := nat.PortSet{
		nat.Port("80/tcp"): struct{}{},
	}

	cfg := &container.Config{
		Image:        volumeBrowseImage,
		User:         "0:0",
		Env:          env,
		ExposedPorts: exposed,
		Labels: map[string]string{
			"tradis.managed": "true",
			"tradis.role":    "volume-browser",
			"tradis.session": sid,
			"tradis.volume":  volumeName,
		},
	}
	hostCfg := &container.HostConfig{
		PortBindings: nat.PortMap{
			nat.Port("80/tcp"): []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}},
		},
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeVolume,
				Source:   volumeName,
				Target:   "/srv",
				ReadOnly: readOnly,
			},
		},
	}

	createResp, err := cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, containerName)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "create helper container failed", err)
		return
	}
	if err := cli.ContainerStart(ctx, createResp.ID, types.ContainerStartOptions{}); err != nil {
		_ = cli.ContainerRemove(ctx, createResp.ID, types.ContainerRemoveOptions{Force: true})
		respondError(c, http.StatusInternalServerError, "start helper container failed", err)
		return
	}

	targetHost := ""
	for i := 0; i < 15; i++ {
		ins, err := cli.ContainerInspect(ctx, createResp.ID)
		if err == nil {
			targetHost = pickContainerHost(ins)
		}
		if targetHost != "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if targetHost == "" {
		_ = cli.ContainerRemove(ctx, createResp.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		respondError(c, http.StatusInternalServerError, "resolve helper container address failed", nil)
		return
	}
	if err := waitForVolumeBrowserReady(ctx, cli, createResp.ID, targetHost); err != nil {
		_ = cli.ContainerRemove(ctx, createResp.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		respondError(c, http.StatusBadGateway, "helper container not ready", err)
		return
	}

	volumeBrowseMu.Lock()
	volumeBrowseSessions[sid] = &volumeBrowseSession{
		ID:          sid,
		VolumeName:  volumeName,
		ContainerID: createResp.ID,
		TargetHost:  targetHost,
		ReadOnly:    readOnly,
		LastSeen:    time.Now(),
	}
	volumeBrowseMu.Unlock()

	volumeBrowseOnce.Do(startVolumeBrowseReaper)

	_ = database.SaveNotification(&database.Notification{
		Type:     "info",
		Category: "system",
		Message:  fmt.Sprintf("卷文件浏览已启动：%s", volumeName),
		Read:     false,
	})

	c.JSON(http.StatusOK, gin.H{
		"sessionId": sid,
		"url":       "/api/volumes/browse/" + sid + "/ui",
		"readOnly":  readOnly,
	})
}

func volumeBrowseUI(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	sid := strings.TrimSpace(c.Param("sid"))
	if !validateSessionID(sid) {
		respondError(c, http.StatusBadRequest, "invalid session id", nil)
		return
	}
	s := getVolumeBrowseSession(sid)
	if s == nil {
		respondError(c, http.StatusNotFound, "session not found", nil)
		return
	}
	tp := "/api/volumes/browse/" + sid + "/fb/"
	hb := "/api/volumes/browse/" + sid + "/heartbeat"
	cl := "/api/volumes/browse/" + sid + "/close"

	html := `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' fill='%230b0f19'/%3E%3Ctext x='16' y='21' font-size='14' text-anchor='middle' fill='%23e5e7eb'%3EFB%3C/text%3E%3C/svg%3E" />
  <title>卷文件浏览器</title>
  <style>
    html, body { height: 100%; margin: 0; background: #0b0f19; }
    .bar { height: 44px; display:flex; align-items:center; padding:0 12px; color:#e5e7eb; font: 14px system-ui, -apple-system, Segoe UI, Roboto, sans-serif; }
    .bar .hint { opacity: 0.75; }
    iframe { width: 100%; height: calc(100% - 44px); border: 0; background: #fff; }
  </style>
</head>
<body>
  <div class="bar">
    <div class="hint">关闭此页面将自动清理临时容器。</div>
  </div>
  <iframe id="fb"></iframe>
  <script>
    const iframe = document.getElementById('fb');
    const iframeUrl = new URL('` + tp + `', location.origin);
    iframe.src = iframeUrl.toString();

    const hbUrl = new URL('` + hb + `', location.origin);
    const closeUrl = new URL('` + cl + `', location.origin);

    const tick = () => fetch(hbUrl.toString(), { method: 'POST', keepalive: true, credentials: 'same-origin' }).catch(() => {});
    tick();
    const timer = setInterval(tick, 15000);

    const close = () => {
      clearInterval(timer);
      try {
        if (navigator.sendBeacon) {
          navigator.sendBeacon(closeUrl.toString(), '');
        } else {
          fetch(closeUrl.toString(), { method: 'POST', keepalive: true, credentials: 'same-origin' }).catch(() => {});
        }
      } catch (e) {}
    };
    window.addEventListener('beforeunload', close);
  </script>
</body>
</html>`

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, html)
}

func volumeBrowseHeartbeat(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	sid := strings.TrimSpace(c.Param("sid"))
	if !validateSessionID(sid) {
		respondError(c, http.StatusBadRequest, "invalid session id", nil)
		return
	}
	volumeBrowseMu.Lock()
	if s, ok := volumeBrowseSessions[sid]; ok && s != nil {
		s.LastSeen = time.Now()
	}
	volumeBrowseMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func volumeBrowseClose(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	sid := strings.TrimSpace(c.Param("sid"))
	if !validateSessionID(sid) {
		respondError(c, http.StatusBadRequest, "invalid session id", nil)
		return
	}
	_ = closeVolumeBrowseSession(sid)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func volumeBrowseProxy(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	sid := strings.TrimSpace(c.Param("sid"))
	if !validateSessionID(sid) {
		respondError(c, http.StatusBadRequest, "invalid session id", nil)
		return
	}

	s := getVolumeBrowseSession(sid)
	if s == nil {
		respondError(c, http.StatusNotFound, "session not found", nil)
		return
	}
	volumeBrowseMu.Lock()
	s.LastSeen = time.Now()
	volumeBrowseMu.Unlock()

	subPath := c.Param("path")
	if strings.TrimSpace(subPath) == "" {
		subPath = "/"
	}
	basePrefix := "/api/volumes/browse/" + sid + "/fb/"
	basePrefixNoSlash := strings.TrimSuffix(basePrefix, "/")

	target := &url.URL{Scheme: "http", Host: s.TargetHost}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = subPath
		req.URL.RawPath = ""
		req.Host = target.Host
		req.Header.Del("X-Forwarded-Host")
		req.Header.Del("X-Forwarded-Proto")
		req.Header.Del("X-Forwarded-For")
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
		isHTML := strings.Contains(ct, "text/html")
		if !isHTML {
			return nil
		}

		p := ""
		if resp.Request != nil && resp.Request.URL != nil {
			p = strings.ToLower(strings.TrimSpace(resp.Request.URL.Path))
		}
		if strings.HasSuffix(p, ".js") || strings.HasSuffix(p, ".mjs") || strings.HasSuffix(p, ".css") {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
			_ = resp.Body.Close()

			msg := fmt.Sprintf("unexpected html response for asset path: %s (upstream=%s, status=%d)", p, s.TargetHost, resp.StatusCode)
			if len(body) > 0 {
				msg += "\n\n" + string(body)
			}

			resp.StatusCode = http.StatusBadGateway
			resp.Status = http.StatusText(resp.StatusCode)
			resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
			resp.Body = io.NopCloser(strings.NewReader(msg))
			resp.ContentLength = int64(len(msg))
			resp.Header.Del("Content-Length")
			return nil
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		if err != nil {
			return err
		}
		_ = resp.Body.Close()

		s := string(body)
		s = strings.ReplaceAll(s, "\"BaseURL\":\"\"", "\"BaseURL\":\""+basePrefixNoSlash+"\"")
		s = strings.ReplaceAll(s, "\"StaticURL\":\"/static\"", "\"StaticURL\":\""+basePrefixNoSlash+"/static\"")
		s = strings.ReplaceAll(s, "\"LogoutPage\":\"/login\"", "\"LogoutPage\":\""+basePrefixNoSlash+"/login\"")
		s = strings.ReplaceAll(s, "\"/static/", "\""+basePrefixNoSlash+"/static/")
		s = strings.ReplaceAll(s, "'/static/", "'"+basePrefixNoSlash+"/static/")
		s = strings.ReplaceAll(s, "src=/static/", "src="+basePrefixNoSlash+"/static/")
		s = strings.ReplaceAll(s, "href=/static/", "href="+basePrefixNoSlash+"/static/")

		s = strings.ReplaceAll(s, "\"/assets/", "\""+basePrefixNoSlash+"/assets/")
		s = strings.ReplaceAll(s, "'/assets/", "'"+basePrefixNoSlash+"/assets/")
		s = strings.ReplaceAll(s, "src=/assets/", "src="+basePrefixNoSlash+"/assets/")
		s = strings.ReplaceAll(s, "href=/assets/", "href="+basePrefixNoSlash+"/assets/")

		s = strings.ReplaceAll(s, "\"/favicon", "\""+basePrefixNoSlash+"/favicon")
		s = strings.ReplaceAll(s, "'/favicon", "'"+basePrefixNoSlash+"/favicon")

		resp.Body = io.NopCloser(strings.NewReader(s))
		resp.ContentLength = int64(len(s))
		resp.Header.Del("Content-Length")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		if status := inspectVolumeBrowseContainerStatus(s.ContainerID); status != "" {
			err = fmt.Errorf("%w; helper status: %s", err, status)
		}
		respondError(c, http.StatusBadGateway, "proxy failed", err)
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func waitForVolumeBrowserReady(ctx context.Context, cli *docker.Client, containerID, targetHost string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	httpClient := &http.Client{Timeout: 1200 * time.Millisecond}
	url := "http://" + targetHost + "/"
	var lastErr error

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("timeout waiting for file browser at %s: %w; logs: %s", targetHost, lastErr, readVolumeBrowseContainerLogs(containerID))
			}
			return fmt.Errorf("timeout waiting for file browser at %s; logs: %s", targetHost, readVolumeBrowseContainerLogs(containerID))
		default:
		}

		ins, err := cli.ContainerInspect(ctx, containerID)
		if err != nil {
			lastErr = err
		} else if ins.State != nil {
			if !ins.State.Running {
				return fmt.Errorf("helper container exited with code %d: %s; logs: %s", ins.State.ExitCode, strings.TrimSpace(ins.State.Error), readVolumeBrowseContainerLogs(containerID))
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := httpClient.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16*1024))
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
			lastErr = fmt.Errorf("http status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func inspectVolumeBrowseContainerStatus(containerID string) string {
	if strings.TrimSpace(containerID) == "" {
		return ""
	}
	cli, err := docker.NewDockerClient()
	if err != nil {
		return ""
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ins, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return err.Error()
	}

	parts := []string{}
	if ins.State != nil {
		parts = append(parts, "state="+strings.TrimSpace(ins.State.Status))
		if ins.State.Health != nil {
			parts = append(parts, "health="+strings.TrimSpace(ins.State.Health.Status))
		}
		if !ins.State.Running {
			parts = append(parts, fmt.Sprintf("exitCode=%d", ins.State.ExitCode))
		}
	}
	if logs := readVolumeBrowseContainerLogs(containerID); logs != "" {
		parts = append(parts, "logs="+logs)
	}
	return strings.Join(parts, "; ")
}

func readVolumeBrowseContainerLogs(containerID string) string {
	if strings.TrimSpace(containerID) == "" {
		return ""
	}
	cli, err := docker.NewDockerClient()
	if err != nil {
		return ""
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rc, err := cli.ContainerLogs(ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "30",
	})
	if err != nil {
		return err.Error()
	}
	defer rc.Close()

	b, err := io.ReadAll(io.LimitReader(rc, 16*1024))
	if err != nil {
		return err.Error()
	}
	logs := strings.TrimSpace(string(b))
	if len(logs) > 2000 {
		logs = logs[len(logs)-2000:]
	}
	return logs
}

func startVolumeBrowseReaper() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			expired := make([]string, 0, 8)
			expiredSet := map[string]struct{}{}
			now := time.Now()
			snapshot := map[string]*volumeBrowseSession{}
			volumeBrowseMu.Lock()
			for sid, s := range volumeBrowseSessions {
				snapshot[sid] = s
				if s == nil {
					expired = append(expired, sid)
					expiredSet[sid] = struct{}{}
					continue
				}
				if now.Sub(s.LastSeen) > volumeBrowseTTL {
					expired = append(expired, sid)
					expiredSet[sid] = struct{}{}
				}
			}
			volumeBrowseMu.Unlock()
			for _, sid := range expired {
				_ = closeVolumeBrowseSession(sid)
			}
			cleanupOrphanVolumeBrowsers(snapshot, expiredSet)
		}
	}()
}

func cleanupOrphanVolumeBrowsers(sessions map[string]*volumeBrowseSession, expired map[string]struct{}) {
	cli, err := docker.NewDockerClient()
	if err != nil {
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	args := filters.NewArgs(
		filters.Arg("label", "tradis.role=volume-browser"),
	)
	list, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true, Filters: args})
	if err != nil {
		return
	}
	for _, c := range list {
		sid := strings.TrimSpace(c.Labels["tradis.session"])
		if sid == "" {
			_ = cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
			continue
		}
		if _, ok := expired[sid]; ok {
			_ = cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
			continue
		}
		s := sessions[sid]
		if s == nil {
			_ = cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
			continue
		}
		if s.ContainerID != "" && s.ContainerID != c.ID {
			_ = cli.ContainerRemove(ctx, c.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
			continue
		}
	}
}

func closeVolumeBrowseSession(sid string) error {
	s := (*volumeBrowseSession)(nil)
	volumeBrowseMu.Lock()
	if v, ok := volumeBrowseSessions[sid]; ok {
		s = v
		delete(volumeBrowseSessions, sid)
	}
	volumeBrowseMu.Unlock()
	if s == nil {
		return nil
	}

	cli, err := docker.NewDockerClient()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = cli.ContainerRemove(ctx, s.ContainerID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		cli.Close()
	}

	_ = database.SaveNotification(&database.Notification{
		Type:     "info",
		Category: "system",
		Message:  fmt.Sprintf("卷文件浏览已关闭：%s", s.VolumeName),
		Read:     false,
	})
	return nil
}

func getVolumeBrowseSession(sid string) *volumeBrowseSession {
	volumeBrowseMu.Lock()
	defer volumeBrowseMu.Unlock()
	s := volumeBrowseSessions[sid]
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

func newSessionID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// validateSessionID 验证 Session ID 格式：必须是24字符的十六进制字符串
func validateSessionID(sid string) bool {
	if len(sid) != 24 {
		return false
	}
	_, err := hex.DecodeString(sid)
	return err == nil
}

func pickContainerHost(ins types.ContainerJSON) string {
	if ins.NetworkSettings != nil {
		if ins.NetworkSettings.Ports != nil {
			if bindings, ok := ins.NetworkSettings.Ports[nat.Port("80/tcp")]; ok && len(bindings) > 0 {
				hostPort := strings.TrimSpace(bindings[0].HostPort)
				if hostPort != "" {
					return "127.0.0.1:" + hostPort
				}
			}
		}
		if ip := strings.TrimSpace(ins.NetworkSettings.IPAddress); ip != "" {
			return ip + ":80"
		}
		for _, n := range ins.NetworkSettings.Networks {
			if n == nil {
				continue
			}
			if ip := strings.TrimSpace(n.IPAddress); ip != "" {
				return ip + ":80"
			}
		}
	}
	return ""
}

func ensureImage(ctx context.Context, cli *docker.Client, image string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if _, _, err := cli.ImageInspectWithRaw(ctx, image); err == nil {
		return nil
	}
	rc, err := cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(rc, 2*1024*1024))
	return nil
}

func isVolumeInUse(ctx context.Context, cli *docker.Client, volName string) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return true
	}
	for _, ctr := range containers {
		for _, m := range ctr.Mounts {
			if m.Type == "volume" && strings.TrimSpace(m.Name) == volName {
				return true
			}
		}
	}
	return false
}

func requireAdmin(c *gin.Context) bool {
	u := strings.TrimSpace(c.GetString("username"))
	if u == "admin" {
		return true
	}
	respondError(c, http.StatusForbidden, "管理员权限 required", nil)
	return false
}
