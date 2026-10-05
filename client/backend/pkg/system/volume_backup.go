package system

import (
	"context"
	"crypto/sha256"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/secrets"
	"dockerpanel/backend/pkg/settings"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
)

const volumeBackupContainerName = "tradis-volume-backup"

var volumeBackupDockerClientMu sync.RWMutex
var volumeBackupReconcileMu sync.Mutex
var newVolumeBackupDockerClient = docker.NewDockerClient

type volumeBackupRuntimeHooks struct {
	ensureImageReady func(context.Context, *docker.Client, string) error
	findContainer    func(context.Context, *docker.Client) (string, string, bool)
	createContainer  func(context.Context, *docker.Client, *container.Config, *container.HostConfig, string) (container.CreateResponse, error)
	startContainer   func(context.Context, *docker.Client, string) error
	removeContainer  func(context.Context, *docker.Client, string) error
	renameContainer  func(context.Context, *docker.Client, string, string) error
	stopContainer    func(context.Context, *docker.Client, string) error
	saveNotification func(string, string)
}

var volumeBackupHooksMu sync.RWMutex
var volumeBackupHooks = volumeBackupRuntimeHooks{
	ensureImageReady: ensureImageReady,
	findContainer:    findVolumeBackupContainer,
	createContainer: func(ctx context.Context, cli *docker.Client, cfg *container.Config, hostCfg *container.HostConfig, name string) (container.CreateResponse, error) {
		return cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	},
	startContainer: func(ctx context.Context, cli *docker.Client, id string) error {
		return cli.ContainerStart(ctx, id, types.ContainerStartOptions{})
	},
	removeContainer: func(ctx context.Context, cli *docker.Client, id string) error {
		return cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true, RemoveVolumes: false})
	},
	renameContainer: func(ctx context.Context, cli *docker.Client, id, name string) error {
		return cli.ContainerRename(ctx, id, name)
	},
	stopContainer: func(ctx context.Context, cli *docker.Client, id string) error {
		return cli.ContainerStop(ctx, id, container.StopOptions{})
	},
	saveNotification: saveNotification,
}

type legacyVolumeBackupSpec struct {
	Image  string
	Env    []string
	Mounts []mount.Mount
	Binds  []string
}

func EnsureVolumeBackupContainer(s settings.Settings) {
	EnsureVolumeBackupContainerContext(context.Background(), s)
}

func EnsureVolumeBackupContainerContext(parent context.Context, s settings.Settings) {
	ensureVolumeBackupContainer(parent, s, false)
}

func RebuildVolumeBackupContainer(s settings.Settings) {
	ensureVolumeBackupContainer(context.Background(), s, true)
}

func ensureVolumeBackupContainer(parent context.Context, s settings.Settings, force bool) {
	volumeBackupReconcileMu.Lock()
	defer volumeBackupReconcileMu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return
	}
	hooks := currentVolumeBackupRuntimeHooks()

	volumeBackupDockerClientMu.RLock()
	newDockerClient := newVolumeBackupDockerClient
	volumeBackupDockerClientMu.RUnlock()
	cli, err := newDockerClient()
	if err != nil {
		saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "error", "卷备份容器启用失败：Docker 客户端初始化失败")
		return
	}
	defer cli.Close()
	if ctx.Err() != nil {
		return
	}

	if !s.VolumeBackupEnabled {
		_ = removeContainerByName(ctx, cli, volumeBackupContainerName)
		if ctx.Err() != nil {
			return
		}
		return
	}

	vols := normalizeStringList(s.VolumeBackupVolumes)
	if len(vols) == 0 {
		_ = removeContainerByName(ctx, cli, volumeBackupContainerName)
		if ctx.Err() != nil {
			return
		}
		saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "warning", "卷备份未启用：未选择需要备份的卷")
		return
	}

	image := strings.TrimSpace(s.VolumeBackupImage)
	if image == "" {
		image = "offen/docker-volume-backup:latest"
	}

	if err := hooks.ensureImageReady(ctx, cli, image); err != nil {
		if ctx.Err() != nil {
			return
		}
		saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "error", "卷备份镜像拉取失败："+redactDockerError(err))
		return
	}
	if ctx.Err() != nil {
		return
	}

	backupEnv, err := settings.GetVolumeBackupEnv()
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "warning", "卷备份环境变量无法解密，请重新输入后保存")
		return
	}
	spec, err := buildLegacyVolumeBackupSpec(image, vols, backupEnv, s.VolumeBackupCronExpression, s.VolumeBackupArchiveDir, s.VolumeBackupMountDockerSock)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "error", "卷备份归档目录必须是宿主机绝对路径："+strings.TrimSpace(s.VolumeBackupArchiveDir))
		return
	}
	archiveDir := strings.TrimSpace(s.VolumeBackupArchiveDir)
	if archiveDir != "" {
		archiveDir = filepath.Clean(archiveDir)
		if err := os.MkdirAll(archiveDir, 0755); err != nil {
			if ctx.Err() != nil {
				return
			}
			saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "warning", "卷备份归档目录无法由当前进程预创建，将交由 Docker 创建："+redactDockerError(err))
		}
		if ctx.Err() != nil {
			return
		}
	}

	cfg := &container.Config{
		Image: spec.Image,
		Env:   spec.Env,
		Labels: map[string]string{
			"tradis.managed":   "true",
			"tradis.role":      "volume-backup",
			"tradis.spec_hash": buildVolumeBackupSpecHash(spec.Image, spec.Env, spec.Mounts, spec.Binds),
		},
	}
	hostCfg := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "always"},
		Mounts:        spec.Mounts,
		Binds:         spec.Binds,
	}

	existingID, existingHash, existingRunning := hooks.findContainer(ctx, cli)
	if ctx.Err() != nil {
		return
	}
	desiredHash := cfg.Labels["tradis.spec_hash"]
	if !force && existingID != "" && existingHash != "" && desiredHash != "" && existingHash == desiredHash {
		if existingRunning {
			return
		}
	}
	if existingID != "" {
		if err := replaceVolumeBackupContainer(ctx, cli, hooks, existingID, existingRunning, cfg, hostCfg); err != nil {
			logging.Error("volume backup replacement failed", "error", redactDockerError(err))
			saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "error", "卷备份容器替换失败："+redactDockerError(err))
		} else {
			saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "success", "卷备份已启用（docker-volume-backup）")
		}
		return
	}

	resp, err := hooks.createContainer(ctx, cli, cfg, hostCfg, volumeBackupContainerName)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "error", "卷备份容器创建失败："+redactDockerError(err))
		return
	}

	if err := hooks.startContainer(ctx, cli, resp.ID); err != nil {
		if ctx.Err() != nil {
			return
		}
		saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "error", "卷备份容器启动失败："+redactDockerError(err))
		if ctx.Err() != nil {
			return
		}
		_ = hooks.removeContainer(ctx, cli, resp.ID)
		return
	}
	if ctx.Err() != nil {
		return
	}

	saveVolumeBackupNotificationContext(ctx, hooks.saveNotification, "success", "卷备份已启用（docker-volume-backup）")
}

func replaceVolumeBackupContainer(ctx context.Context, cli *docker.Client, hooks volumeBackupRuntimeHooks, oldID string, wasRunning bool, cfg *container.Config, hostCfg *container.HostConfig) (err error) {
	backupName := volumeBackupContainerName + "-previous-" + oldID
	if err := hooks.renameContainer(ctx, cli, oldID, backupName); err != nil {
		return err
	}
	newID := ""
	stopAttempted := false
	defer func() {
		if err == nil {
			return
		}
		// Recovery must survive cancellation of the configuration request.
		recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if newID != "" {
			err = errors.Join(err, hooks.removeContainer(recovery, cli, newID))
		}
		err = errors.Join(err, hooks.renameContainer(recovery, cli, oldID, volumeBackupContainerName))
		if stopAttempted && wasRunning {
			err = errors.Join(err, hooks.startContainer(recovery, cli, oldID))
		}
	}()
	created, err := hooks.createContainer(ctx, cli, cfg, hostCfg, volumeBackupContainerName)
	newID = created.ID
	if err != nil {
		return err
	}
	if wasRunning {
		stopAttempted = true
		if err = hooks.stopContainer(ctx, cli, oldID); err != nil {
			return err
		}
	}
	if err = hooks.startContainer(ctx, cli, newID); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if cleanupErr := hooks.removeContainer(ctx, cli, oldID); cleanupErr != nil {
		logging.Warn("new volume backup container started; previous container retained", "error", redactDockerError(cleanupErr))
	}
	return nil
}

func currentVolumeBackupRuntimeHooks() volumeBackupRuntimeHooks {
	volumeBackupHooksMu.RLock()
	hooks := volumeBackupHooks
	volumeBackupHooksMu.RUnlock()
	return hooks
}

func saveVolumeBackupNotificationContext(ctx context.Context, save func(string, string), tp, msg string) {
	if ctx == nil || ctx.Err() != nil {
		return
	}
	save(tp, msg)
}

// buildLegacyVolumeBackupSpec keeps the existing global volume-backup
// container as a separate transport primitive. The application protection
// workflow can opt into it later without rewriting legacy settings.
func buildLegacyVolumeBackupSpec(image string, volumes []string, backupEnv, cronExpression, archiveDir string, mountDockerSock bool) (legacyVolumeBackupSpec, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		image = "offen/docker-volume-backup:latest"
	}
	volumes = normalizeStringList(volumes)
	spec := legacyVolumeBackupSpec{
		Image:  image,
		Env:    normalizeEnvList(upsertEnv(parseEnvText(backupEnv), "BACKUP_CRON_EXPRESSION", normalizeBackupCronExpression(cronExpression))),
		Mounts: make([]mount.Mount, 0, len(volumes)+1),
		Binds:  make([]string, 0, 1),
	}
	for _, volume := range volumes {
		spec.Mounts = append(spec.Mounts, mount.Mount{Type: mount.TypeVolume, Source: volume, Target: "/backup/" + sanitizeVolumeTarget(volume), ReadOnly: true})
	}
	if mountDockerSock {
		spec.Mounts = append(spec.Mounts, mount.Mount{Type: mount.TypeBind, Source: "/var/run/docker.sock", Target: "/var/run/docker.sock", ReadOnly: true})
	}
	archiveDir = strings.TrimSpace(archiveDir)
	if archiveDir != "" {
		archiveDir = filepath.Clean(archiveDir)
		if !filepath.IsAbs(archiveDir) {
			return legacyVolumeBackupSpec{}, fmt.Errorf("archive directory must be absolute")
		}
		spec.Binds = append(spec.Binds, archiveDir+":/archive")
	}
	return spec, nil
}

func normalizeEnvList(env []string) []string {
	next := make([]string, 0, len(env))
	for _, e := range env {
		s := strings.TrimSpace(e)
		if s == "" {
			continue
		}
		next = append(next, s)
	}
	sort.Strings(next)
	return next
}

func buildVolumeBackupSpecHash(image string, env []string, mounts []mount.Mount, binds []string) string {
	var b strings.Builder
	b.WriteString("image=")
	b.WriteString(strings.TrimSpace(image))
	b.WriteString("\n")
	for _, e := range env {
		b.WriteString("env=")
		b.WriteString(strings.TrimSpace(e))
		b.WriteString("\n")
	}
	for _, m := range mounts {
		b.WriteString("mnt=")
		b.WriteString(string(m.Type))
		b.WriteString("|")
		b.WriteString(strings.TrimSpace(m.Source))
		b.WriteString("|")
		b.WriteString(strings.TrimSpace(m.Target))
		b.WriteString("|")
		if m.ReadOnly {
			b.WriteString("ro")
		} else {
			b.WriteString("rw")
		}
		b.WriteString("\n")
	}
	for _, bind := range binds {
		b.WriteString("bind=")
		b.WriteString(strings.TrimSpace(bind))
		b.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func findVolumeBackupContainer(ctx context.Context, cli *docker.Client) (id string, specHash string, running bool) {
	args := filters.NewArgs(filters.Arg("name", volumeBackupContainerName))
	list, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true, Filters: args})
	if err != nil || len(list) == 0 {
		return "", "", false
	}
	for _, c := range list {
		found := false
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == volumeBackupContainerName {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		id = c.ID
		running = strings.ToLower(strings.TrimSpace(c.State)) == "running"
		if c.Labels != nil {
			specHash = strings.TrimSpace(c.Labels["tradis.spec_hash"])
		}
		return id, specHash, running
	}
	return "", "", false
}

func normalizeBackupCronExpression(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "@daily"
	}
	return v
}

func upsertEnv(env []string, key string, value string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return env
	}
	item := key + "=" + value
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			if !replaced {
				out = append(out, item)
				replaced = true
			}
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, item)
	}
	return out
}

func ensureImageReady(ctx context.Context, cli *docker.Client, image string) error {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil
	}
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

func removeContainerByName(ctx context.Context, cli *docker.Client, name string) error {
	id := ""
	args := filters.NewArgs(filters.Arg("name", name))
	list, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true, Filters: args})
	if err != nil {
		return err
	}
	for _, c := range list {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				id = c.ID
				break
			}
		}
		if id != "" {
			break
		}
	}
	if id == "" {
		return nil
	}
	_ = cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true, RemoveVolumes: false})
	return nil
}

func saveNotification(tp string, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	if tp == "" {
		tp = "info"
	}
	_ = database.SaveNotification(&database.Notification{Type: tp, Category: "volume_backup_task", Message: msg, Read: false})
}

func redactDockerError(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	s = settings.RedactAppStoreURL(s)
	s = logging.RedactText(s)
	s = secrets.RedactString(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func parseEnvText(raw string) []string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	index := make(map[string]int, 32)
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if !strings.Contains(s, "=") {
			continue
		}
		parts := strings.SplitN(s, "=", 2)
		key := strings.TrimSpace(parts[0])
		val := parts[1]
		if key == "" || !envKeyRe.MatchString(key) {
			continue
		}
		item := key + "=" + val
		if idx, ok := index[key]; ok {
			out[idx] = item
			continue
		}
		index[key] = len(out)
		out = append(out, item)
	}
	return out
}

func normalizeStringList(list []string) []string {
	out := make([]string, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, v := range list {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func sanitizeVolumeTarget(v string) string {
	s := strings.TrimSpace(v)
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.TrimSpace(s)
	if s == "" {
		return "volume"
	}
	return s
}
