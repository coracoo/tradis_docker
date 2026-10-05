package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	tradisdocker "dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/errdefs"
	"github.com/gin-gonic/gin"
)

const composeProjectLabel = "com.docker.compose.project"

type composeDestroyDockerClient interface {
	ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
	ContainerRemove(context.Context, string, types.ContainerRemoveOptions) error
	VolumeList(context.Context, volume.ListOptions) (volume.ListResponse, error)
	NetworkList(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error)
	NetworkRemove(context.Context, string) error
	VolumeRemove(context.Context, string, bool) error
	ImageList(context.Context, types.ImageListOptions) ([]types.ImageSummary, error)
	ImageRemove(context.Context, string, types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error)
	Close() error
}

type composeDestroyResource struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
	Reason string `json:"reason,omitempty"`
	ID     string `json:"-"`
}

type composeDestroyInventory struct {
	Project     string                   `json:"project"`
	Fingerprint string                   `json:"fingerprint"`
	Delete      []composeDestroyResource `json:"delete"`
	Retain      []composeDestroyResource `json:"retain"`
}

var composeDestroyDockerClientFactory = func() (composeDestroyDockerClient, error) {
	return tradisdocker.NewDockerClient()
}

var composeDestroyPersistentStateCleaner = deleteComposeProjectPersistentState

type composeDestroyRequest struct {
	Fingerprint string   `json:"fingerprint"`
	Selected    []string `json:"selected"`
}

func previewComposeDestroy(c *gin.Context) {
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}
	inventory, err := loadComposeDestroyInventory(c.Request.Context(), target)
	if err != nil {
		respondError(c, http.StatusBadGateway, "无法生成销毁清单", err)
		return
	}
	c.JSON(http.StatusOK, inventory)
}

func destroyProjectTask(c *gin.Context) {
	var req composeDestroyRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Fingerprint) == "" {
		respondError(c, http.StatusBadRequest, "缺少有效的销毁确认信息", err)
		return
	}
	target, ok := resolveComposeRequestTarget(c)
	if !ok {
		return
	}
	inventory, err := loadComposeDestroyInventory(c.Request.Context(), target)
	if err != nil {
		respondError(c, http.StatusBadGateway, "无法核对销毁清单", err)
		return
	}
	if inventory.Fingerprint != strings.TrimSpace(req.Fingerprint) {
		respondComposeDestroyInventoryStale(c)
		return
	}
	if _, err := selectComposeDestroyResources(inventory, req.Selected); err != nil {
		respondError(c, http.StatusBadRequest, "销毁资源选择无效", err)
		return
	}
	createComposeOperationTask(c, "destroy", composeOperationOptions{
		DestroyFingerprint: inventory.Fingerprint,
		DestroySelected:    append([]string(nil), req.Selected...),
	})
}

func loadComposeDestroyInventory(ctx context.Context, target composeOperationTarget) (composeDestroyInventory, error) {
	cli, err := composeDestroyDockerClientFactory()
	if err != nil {
		return composeDestroyInventory{}, fmt.Errorf("连接 Docker 失败: %w", err)
	}
	defer cli.Close()
	return buildComposeDestroyInventory(ctx, cli, target)
}

func respondComposeDestroyInventoryStale(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{
		"code":    "destroy_inventory_stale",
		"message": "项目资源已发生变化，请刷新清单后重新确认",
	})
}

func buildComposeDestroyInventory(
	ctx context.Context,
	cli composeDestroyDockerClient,
	target composeOperationTarget,
) (composeDestroyInventory, error) {
	if cli == nil {
		return composeDestroyInventory{}, fmt.Errorf("Docker 客户端不可用")
	}
	projectName := strings.TrimSpace(target.ComposeProjectName)
	projectDir := filepath.Clean(strings.TrimSpace(target.ProjectDir))
	if !isExactComposeProjectName(projectName) || projectDir == "." || !filepath.IsAbs(projectDir) {
		return composeDestroyInventory{}, fmt.Errorf("Compose 项目身份不完整")
	}

	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return composeDestroyInventory{}, fmt.Errorf("枚举容器失败: %w", err)
	}
	volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return composeDestroyInventory{}, fmt.Errorf("枚举数据卷失败: %w", err)
	}
	networks, err := cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return composeDestroyInventory{}, fmt.Errorf("枚举网络失败: %w", err)
	}

	projectContainers := make([]types.Container, 0)
	otherImageUse := make(map[string]bool)
	otherVolumeUse := make(map[string]bool)
	for _, container := range containers {
		if strings.TrimSpace(container.Labels[composeProjectLabel]) == projectName {
			projectContainers = append(projectContainers, container)
			continue
		}
		if key := composeDestroyImageKey(container); key != "" {
			otherImageUse[key] = true
		}
		for _, mount := range container.Mounts {
			if mount.Type == "volume" && strings.TrimSpace(mount.Name) != "" {
				otherVolumeUse[strings.TrimSpace(mount.Name)] = true
			}
		}
	}

	volumeByName := make(map[string]*volume.Volume, len(volumes.Volumes))
	for _, item := range volumes.Volumes {
		if item != nil {
			volumeByName[strings.TrimSpace(item.Name)] = item
		}
	}

	inventory := composeDestroyInventory{Project: firstNonEmptyComposeDestroyString(target.DisplayName, projectName)}
	projectDirDisplay := projectDir
	projectDirDetail := "Compose 配置和项目目录（容器内路径）"
	if hostRoot := strings.TrimSpace(effectiveHostProjectRoot()); hostRoot != "" && target.RelativePath != "" {
		projectDirDisplay = filepath.Join(hostRoot, filepath.FromSlash(target.RelativePath))
		projectDirDetail = "Compose 配置和项目目录（宿主机路径）"
	}
	inventory.Delete = append(inventory.Delete, composeDestroyResource{
		Kind: "project_directory", Name: projectDirDisplay, Detail: projectDirDetail, ID: projectDir,
	})
	inventory.Delete = append(inventory.Delete, composeDestroyResource{
		Kind: "project_record", Name: inventory.Project, Detail: "备注、Git 来源、配置历史和端口预留", ID: projectName,
	})
	seenDelete := make(map[string]bool)
	seenRetain := make(map[string]bool)
	projectImages := make(map[string]composeDestroyResource)
	// Failed builds may leave labelled images before any container is created.
	// Labels prove ownership; matching a project-like tag alone does not.
	buildImages, err := cli.ImageList(ctx, types.ImageListOptions{
		All: true, Filters: filters.NewArgs(filters.Arg("label", composeProjectLabel+"="+projectName)),
	})
	if err != nil {
		return composeDestroyInventory{}, fmt.Errorf("枚举项目构建镜像失败: %w", err)
	}
	for _, image := range buildImages {
		if strings.TrimSpace(image.Labels[composeProjectLabel]) != projectName || strings.TrimSpace(image.ID) == "" {
			continue
		}
		names := make([]string, 0, len(image.RepoTags))
		for _, tag := range image.RepoTags {
			if tag = strings.TrimSpace(tag); tag != "" && tag != "<none>:<none>" {
				names = append(names, tag)
			}
		}
		sort.Strings(names)
		name := shortComposeDestroyID(image.ID)
		if len(names) > 0 {
			name = names[0]
		}
		projectImages[image.ID] = composeDestroyResource{
			Kind: "image", Name: name, Detail: shortComposeDestroyID(image.ID), ID: image.ID,
		}
	}
	for _, container := range projectContainers {
		containerName := composeContainerDisplayName(container)
		inventory.Delete = appendUniqueComposeDestroyResource(inventory.Delete, seenDelete, composeDestroyResource{
			Kind: "container", Name: containerName, Detail: shortComposeDestroyID(container.ID), ID: container.ID,
		})
		imageKey := composeDestroyImageKey(container)
		if imageKey != "" {
			image := composeDestroyResource{
				Kind: "image", Name: firstNonEmptyComposeDestroyString(container.Image, shortComposeDestroyID(container.ImageID)),
				Detail: shortComposeDestroyID(container.ImageID), ID: firstNonEmptyComposeDestroyString(container.ImageID, container.Image),
			}
			if previous, exists := projectImages[imageKey]; !exists || image.Name < previous.Name {
				projectImages[imageKey] = image
			}
		}
		for _, mount := range container.Mounts {
			switch mount.Type {
			case "bind":
				source := filepath.Clean(strings.TrimSpace(mount.Source))
				if source == "." || source == "" {
					continue
				}
				// Docker reports host paths. Validate the corresponding readable path
				// after establishing ownership under this project's host directory.
				if relative, inside := pathRelativeToRoot(projectDirDisplay, source); inside &&
					validatePathWithinRoot(projectDir, filepath.Join(projectDir, relative)) == nil {
					continue
				}
				inventory.Retain = appendUniqueComposeDestroyResource(inventory.Retain, seenRetain, composeDestroyResource{
					Kind: "bind", Name: source, Detail: mount.Destination,
					Reason: "项目目录外的宿主机目录，避免误删用户数据", ID: source,
				})
			case "volume":
				name := strings.TrimSpace(mount.Name)
				if name == "" {
					continue
				}
				item := volumeByName[name]
				owned := item != nil && strings.TrimSpace(item.Labels[composeProjectLabel]) == projectName
				if owned && !otherVolumeUse[name] {
					inventory.Delete = appendUniqueComposeDestroyResource(inventory.Delete, seenDelete, composeDestroyResource{
						Kind: "volume", Name: name, Detail: mount.Destination, ID: name,
					})
				} else {
					reason := "外部数据卷"
					if otherVolumeUse[name] {
						reason = "仍被其他容器使用"
					}
					inventory.Retain = appendUniqueComposeDestroyResource(inventory.Retain, seenRetain, composeDestroyResource{
						Kind: "volume", Name: name, Detail: mount.Destination, Reason: reason, ID: name,
					})
				}
			}
		}
	}

	// 同属项目但当前未挂载的 Compose 卷也属于销毁范围。
	for _, item := range volumes.Volumes {
		if item == nil || strings.TrimSpace(item.Labels[composeProjectLabel]) != projectName {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if otherVolumeUse[name] {
			inventory.Retain = appendUniqueComposeDestroyResource(inventory.Retain, seenRetain, composeDestroyResource{
				Kind: "volume", Name: name, Reason: "仍被其他容器使用", ID: name,
			})
			continue
		}
		inventory.Delete = appendUniqueComposeDestroyResource(inventory.Delete, seenDelete, composeDestroyResource{
			Kind: "volume", Name: name, ID: name,
		})
	}

	for key, image := range projectImages {
		if otherImageUse[key] {
			image.Reason = "仍被其他容器使用"
			inventory.Retain = appendUniqueComposeDestroyResource(inventory.Retain, seenRetain, image)
			continue
		}
		inventory.Delete = appendUniqueComposeDestroyResource(inventory.Delete, seenDelete, image)
	}
	for _, network := range networks {
		if strings.TrimSpace(network.Labels[composeProjectLabel]) != projectName {
			continue
		}
		inventory.Delete = appendUniqueComposeDestroyResource(inventory.Delete, seenDelete, composeDestroyResource{
			Kind: "network", Name: network.Name, Detail: shortComposeDestroyID(network.ID), ID: network.ID,
		})
	}

	sortComposeDestroyResources(inventory.Delete)
	sortComposeDestroyResources(inventory.Retain)
	assignComposeDestroyResourceKeys(inventory.Delete)
	assignComposeDestroyResourceKeys(inventory.Retain)
	inventory.Fingerprint = composeDestroyInventoryFingerprint(inventory)
	return inventory, nil
}

type composeDestroySelection struct {
	Delete []composeDestroyResource
	Retain []composeDestroyResource
}

// A nil selection is accepted for clients from before item-level selection and
// preserves their original "delete the complete preview" behavior.
func selectComposeDestroyResources(inventory composeDestroyInventory, selected []string) (composeDestroySelection, error) {
	if selected != nil && len(selected) == 0 {
		return composeDestroySelection{}, fmt.Errorf("至少选择一项要销毁的资源")
	}
	allowed := make(map[string]composeDestroyResource, len(inventory.Delete))
	for _, item := range inventory.Delete {
		allowed[composeDestroyResourceKey(item)] = item
	}
	selectedSet := make(map[string]bool, len(allowed))
	if selected == nil {
		for key := range allowed {
			selectedSet[key] = true
		}
	} else {
		for _, key := range selected {
			key = strings.TrimSpace(key)
			if _, ok := allowed[key]; !ok {
				return composeDestroySelection{}, fmt.Errorf("销毁清单不包含资源 %q", key)
			}
			selectedSet[key] = true
		}
	}
	result := composeDestroySelection{
		Delete: make([]composeDestroyResource, 0, len(selectedSet)),
		Retain: make([]composeDestroyResource, 0, len(allowed)-len(selectedSet)),
	}
	for _, item := range inventory.Delete {
		if selectedSet[composeDestroyResourceKey(item)] {
			result.Delete = append(result.Delete, item)
			continue
		}
		item.Reason = "已由用户取消销毁"
		result.Retain = append(result.Retain, item)
	}
	return result, nil
}

func executeComposeDestroy(
	ctx context.Context,
	target composeOperationTarget,
	expectedFingerprint string,
	selected []string,
	appendLog func(string, string),
) (gin.H, error) {
	cli, err := composeDestroyDockerClientFactory()
	if err != nil {
		return nil, fmt.Errorf("连接 Docker 失败: %w", err)
	}
	defer cli.Close()

	inventory, err := buildComposeDestroyInventory(ctx, cli, target)
	if err != nil {
		return nil, err
	}
	if inventory.Fingerprint != strings.TrimSpace(expectedFingerprint) {
		return nil, fmt.Errorf("销毁清单已变化，请重新打开销毁确认窗口")
	}
	selection, err := selectComposeDestroyResources(inventory, selected)
	if err != nil {
		return nil, err
	}

	deleted := make([]composeDestroyResource, 0, len(selection.Delete))
	retained := append([]composeDestroyResource(nil), inventory.Retain...)
	retained = append(retained, selection.Retain...)
	warnings := make([]string, 0)
	result := func() gin.H {
		return gin.H{"project": target.DisplayName, "deleted": deleted, "retained": retained, "warnings": warnings}
	}
	for _, resource := range selection.Delete {
		if resource.Kind != "container" {
			continue
		}
		removeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		removeErr := cli.ContainerRemove(removeCtx, resource.ID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: false})
		cancel()
		if removeErr != nil && !errdefs.IsNotFound(removeErr) {
			return result(), fmt.Errorf("删除容器 %s 失败: %w", resource.Name, removeErr)
		}
		deleted = append(deleted, resource)
		appendLog("info", "已删除容器 "+resource.Name)
	}
	remainingContainers, err := cli.ContainerList(ctx, types.ContainerListOptions{All: true})
	if err != nil {
		return result(), fmt.Errorf("删除数据卷和镜像前复核容器失败: %w", err)
	}
	usedVolumes := make(map[string]bool)
	usedImages := make(map[string]bool)
	usedNetworks := make(map[string]bool)
	uncheckedProjectContainers := make(map[string]bool)
	for _, resource := range selection.Retain {
		if resource.Kind == "container" {
			uncheckedProjectContainers[resource.ID] = true
		}
	}
	remainingProjectContainers := false
	for _, item := range remainingContainers {
		if strings.TrimSpace(item.Labels[composeProjectLabel]) == target.ComposeProjectName {
			remainingProjectContainers = true
			if !uncheckedProjectContainers[item.ID] {
				return result(), fmt.Errorf("销毁过程中出现或残留了项目容器 %s，请重新核对后处理", composeContainerDisplayName(item))
			}
		}
		if key := composeDestroyImageKey(item); key != "" {
			usedImages[key] = true
		}
		if name := strings.TrimSpace(item.Image); name != "" {
			usedImages[name] = true
		}
		for _, mount := range item.Mounts {
			if mount.Type == "volume" && mount.Name != "" {
				usedVolumes[mount.Name] = true
			}
		}
		if item.NetworkSettings != nil {
			for name, endpoint := range item.NetworkSettings.Networks {
				usedNetworks[name] = true
				if endpoint != nil && endpoint.NetworkID != "" {
					usedNetworks[endpoint.NetworkID] = true
				}
			}
		}
	}
	for _, resource := range selection.Delete {
		if resource.Kind != "network" {
			continue
		}
		if usedNetworks[resource.ID] || usedNetworks[resource.Name] {
			resource.Reason = "销毁执行时检测到仍被保留容器使用"
			retained = append(retained, resource)
			warning := "网络仍被保留容器使用，已保留 " + resource.Name
			warnings = append(warnings, warning)
			appendLog("warning", warning)
			continue
		}
		removeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		removeErr := cli.NetworkRemove(removeCtx, resource.ID)
		cancel()
		if removeErr != nil && !errdefs.IsNotFound(removeErr) {
			return result(), fmt.Errorf("删除网络 %s 失败: %w", resource.Name, removeErr)
		}
		deleted = append(deleted, resource)
		appendLog("info", "已删除网络 "+resource.Name)
	}
	for _, resource := range selection.Delete {
		if resource.Kind != "volume" {
			continue
		}
		if usedVolumes[resource.ID] {
			resource.Reason = "销毁执行时检测到仍被其他容器使用"
			retained = append(retained, resource)
			warning := "数据卷仍被其他容器使用，已保留 " + resource.Name
			warnings = append(warnings, warning)
			appendLog("warning", warning)
			continue
		}
		removeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		removeErr := cli.VolumeRemove(removeCtx, resource.ID, false)
		cancel()
		if removeErr != nil && !errdefs.IsNotFound(removeErr) {
			return result(), fmt.Errorf("删除数据卷 %s 失败: %w", resource.Name, removeErr)
		}
		deleted = append(deleted, resource)
		appendLog("info", "已删除数据卷 "+resource.Name)
	}
	for _, resource := range selection.Delete {
		if resource.Kind != "image" {
			continue
		}
		if usedImages[resource.ID] || usedImages[resource.Name] {
			resource.Reason = "销毁执行时检测到仍被其他容器使用"
			retained = append(retained, resource)
			warning := "镜像仍被其他容器使用，已保留 " + resource.Name
			warnings = append(warnings, warning)
			appendLog("warning", warning)
			continue
		}
		removeCtx, cancel := context.WithTimeout(ctx, time.Minute)
		_, removeErr := cli.ImageRemove(removeCtx, resource.ID, types.ImageRemoveOptions{Force: false, PruneChildren: false})
		cancel()
		if removeErr != nil {
			if errdefs.IsNotFound(removeErr) {
				deleted = append(deleted, resource)
				continue
			}
			if errdefs.IsConflict(removeErr) {
				resource.Reason = "镜像还有其他标签或引用，Docker 拒绝删除"
				retained = append(retained, resource)
				warning := "镜像还有其他引用，已保留 " + resource.Name
				warnings = append(warnings, warning)
				appendLog("warning", warning)
				continue
			}
			return result(), fmt.Errorf("删除镜像 %s 失败: %w", resource.Name, removeErr)
		}
		deleted = append(deleted, resource)
		appendLog("info", "已删除镜像 "+resource.Name)
	}
	deleteDirectory := false
	deleteRecord := false
	for _, resource := range selection.Delete {
		switch resource.Kind {
		case "project_directory":
			deleteDirectory = true
		case "project_record":
			deleteRecord = true
		}
	}
	if remainingProjectContainers {
		for _, resource := range selection.Delete {
			if resource.Kind != "project_directory" && resource.Kind != "project_record" {
				continue
			}
			resource.Reason = "仍有项目容器被保留，项目目录和记录必须保留"
			retained = append(retained, resource)
		}
		deleteDirectory = false
		deleteRecord = false
	}
	if deleteDirectory || deleteRecord {
		if deleteDirectory && deleteRecord {
			if err := composeDestroyPersistentStateCleaner(target); err != nil {
				return result(), err
			}
		} else if err := deleteComposeProjectPersistentStateSelection(target, deleteDirectory, deleteRecord); err != nil {
			return result(), err
		}
		for _, resource := range selection.Delete {
			if (resource.Kind == "project_directory" && deleteDirectory) || (resource.Kind == "project_record" && deleteRecord) {
				deleted = append(deleted, resource)
			}
		}
	}
	sortComposeDestroyResources(deleted)
	sortComposeDestroyResources(retained)
	return result(), nil
}

func deleteComposeProjectPersistentState(target composeOperationTarget) error {
	return deleteComposeProjectPersistentStateSelection(target, true, true)
}

func deleteComposeProjectPersistentStateSelection(target composeOperationTarget, deleteDirectory, deleteRecord bool) error {
	if deleteDirectory {
		if err := os.RemoveAll(target.ProjectDir); err != nil {
			return fmt.Errorf("删除项目目录失败: %w", err)
		}
	}
	if !deleteRecord {
		return nil
	}
	tx, err := database.GetDB().Begin()
	if err != nil {
		return fmt.Errorf("开始清理项目记录失败: %w", err)
	}
	if deleteErr := database.DeleteReservedPortsByOwnerTx(tx, target.ComposeProjectName); deleteErr != nil {
		_ = tx.Rollback()
		return fmt.Errorf("删除项目端口预留失败: %w", deleteErr)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交项目端口清理失败: %w", err)
	}
	if err := database.DeleteComposeGitSourceInEnvironment(database.LocalEnvironmentID, target.DisplayName); err != nil {
		return fmt.Errorf("删除项目 Git 来源失败: %w", err)
	}
	if err := database.DeleteComposeProjectMetadata(database.LocalEnvironmentID, target.DisplayName); err != nil {
		return fmt.Errorf("删除项目备注失败: %w", err)
	}
	if err := database.DeleteComposeHistoryForProject(database.LocalEnvironmentID, target.DisplayName); err != nil {
		return fmt.Errorf("删除项目配置历史失败: %w", err)
	}
	if target.RelativePath != "" {
		if err := database.DeleteComposeProjectIdentity(database.LocalEnvironmentID, target.RelativePath); err != nil {
			return fmt.Errorf("删除项目身份记录失败: %w", err)
		}
	}
	return nil
}

func composeDestroyResourceKey(item composeDestroyResource) string {
	return item.Kind + ":" + firstNonEmptyComposeDestroyString(item.ID, item.Name)
}

func assignComposeDestroyResourceKeys(items []composeDestroyResource) {
	for index := range items {
		items[index].Key = composeDestroyResourceKey(items[index])
	}
}

func composeDestroyImageKey(container types.Container) string {
	if id := strings.TrimSpace(container.ImageID); id != "" {
		return id
	}
	return strings.TrimSpace(container.Image)
}

func shortComposeDestroyID(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "sha256:")
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func appendUniqueComposeDestroyResource(
	items []composeDestroyResource,
	seen map[string]bool,
	item composeDestroyResource,
) []composeDestroyResource {
	key := item.Kind + "\x00" + firstNonEmptyComposeDestroyString(item.ID, item.Name)
	if seen[key] {
		for index := range items {
			if items[index].Kind+"\x00"+firstNonEmptyComposeDestroyString(items[index].ID, items[index].Name) != key {
				continue
			}
			if item.Detail != "" && (items[index].Detail == "" || item.Detail < items[index].Detail) {
				items[index].Detail = item.Detail
			}
			break
		}
		return items
	}
	seen[key] = true
	return append(items, item)
}

func sortComposeDestroyResources(items []composeDestroyResource) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i].Kind + "\x00" + items[i].Name + "\x00" + items[i].ID
		right := items[j].Kind + "\x00" + items[j].Name + "\x00" + items[j].ID
		return left < right
	})
}

func composeDestroyInventoryFingerprint(inventory composeDestroyInventory) string {
	type fingerprintResource struct {
		Kind   string `json:"kind"`
		Name   string `json:"name"`
		Detail string `json:"detail,omitempty"`
		Reason string `json:"reason,omitempty"`
		ID     string `json:"id"`
	}
	convert := func(items []composeDestroyResource) []fingerprintResource {
		result := make([]fingerprintResource, 0, len(items))
		for _, item := range items {
			result = append(result, fingerprintResource{
				Kind: item.Kind, Name: item.Name, Detail: item.Detail, Reason: item.Reason, ID: item.ID,
			})
		}
		return result
	}
	material := struct {
		Project string                `json:"project"`
		Delete  []fingerprintResource `json:"delete"`
		Retain  []fingerprintResource `json:"retain"`
	}{Project: inventory.Project, Delete: convert(inventory.Delete), Retain: convert(inventory.Retain)}
	raw, _ := json.Marshal(material)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func firstNonEmptyComposeDestroyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
