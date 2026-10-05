package api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	tradisdocker "dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/volume"
)

func TestBuildComposeDestroyInventoryMapsHostBindPathsToProject(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "project")
	hostRoot := filepath.Join(root, "nas", "project")
	t.Setenv("PROJECT_ROOT", hostRoot)
	t.Setenv("TRADIS_DATA_DIR", filepath.Join(root, "container", "data"))
	reference := "测试 Name"
	projectDir := filepath.Join(containerRoot, reference)
	hostProjectDir := filepath.Join(hostRoot, reference)
	if err := os.MkdirAll(filepath.Join(projectDir, "test", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	if err := os.Symlink(outsideDir, filepath.Join(projectDir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(projectDir, "test"), filepath.Join(projectDir, "alias")); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		source string
		retain bool
	}{
		{name: "project root", source: hostProjectDir},
		{name: "project child", source: filepath.Join(hostProjectDir, "test")},
		{name: "nested child", source: filepath.Join(hostProjectDir, "test", "nested")},
		{name: "missing child", source: filepath.Join(hostProjectDir, "test", "new")},
		{name: "internal symlink", source: filepath.Join(hostProjectDir, "alias", "nested")},
		{name: "escaping symlink", source: filepath.Join(hostProjectDir, "escape", "data"), retain: true},
		{name: "similar prefix", source: filepath.Join(hostRoot, reference+"-other", "test"), retain: true},
		{name: "other project", source: filepath.Join(hostRoot, "other", "test"), retain: true},
		{name: "parent traversal", source: hostProjectDir + "/../other/test", retain: true},
		{name: "container path is not host project", source: filepath.Join(projectDir, "test"), retain: true},
		{name: "external directory", source: outsideDir, retain: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &tradisdocker.MockDockerClient{
				ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
					return []types.Container{{
						ID: "web", Names: []string{"/demo-web"}, Labels: map[string]string{composeProjectLabel: "demo"},
						Mounts: []types.MountPoint{{Type: "bind", Source: test.source, Destination: "/data"}},
					}}, nil
				},
				VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
					return volume.ListResponse{}, nil
				},
				NetworkListFunc: func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) { return nil, nil },
			}
			inventory, err := buildComposeDestroyInventory(context.Background(), client, composeOperationTarget{
				DisplayName: reference, ComposeProjectName: "demo", ProjectDir: projectDir, RelativePath: reference,
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := destroyResourceNames(inventory.Delete, "project_directory"); !reflect.DeepEqual(got, []string{hostProjectDir}) {
				t.Fatalf("project directory = %#v", got)
			}
			retained := destroyResourceNames(inventory.Retain, "bind")
			if test.retain {
				if !reflect.DeepEqual(retained, []string{filepath.Clean(test.source)}) {
					t.Fatalf("external bind must remain: %#v", retained)
				}
			} else if len(retained) != 0 {
				t.Fatalf("project bind must be covered by project directory deletion, not marked external: %#v", retained)
			}
		})
	}
}

func TestBuildComposeDestroyInventoryClassifiesOwnedAndSharedResources(t *testing.T) {
	projectDir := filepath.Join(t.TempDir(), "demo")
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{
				{
					ID: "project-unique", Names: []string{"/demo-web"}, Image: "example/web:latest", ImageID: "sha256:unique",
					Labels: map[string]string{"com.docker.compose.project": "demo"},
					Mounts: []types.MountPoint{
						{Type: "bind", Source: filepath.Join(projectDir, "data"), Destination: "/data"},
						{Type: "bind", Source: "/volume1/media", Destination: "/media"},
						{Type: "volume", Name: "demo_data", Destination: "/config"},
						{Type: "volume", Name: "shared_external", Destination: "/shared"},
					},
				},
				{ID: "project-shared", Names: []string{"/demo-db"}, Image: "postgres:16", ImageID: "sha256:shared", Labels: map[string]string{"com.docker.compose.project": "demo"}},
				{ID: "other", Names: []string{"/other-db"}, Image: "postgres:16", ImageID: "sha256:shared", Labels: map[string]string{"com.docker.compose.project": "other"}, Mounts: []types.MountPoint{{Type: "volume", Name: "shared_external"}}},
			}, nil
		},
		VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{Volumes: []*volume.Volume{
				{Name: "demo_data", Labels: map[string]string{"com.docker.compose.project": "demo"}},
				{Name: "shared_external"},
			}}, nil
		},
		NetworkListFunc: func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) {
			return []types.NetworkResource{{ID: "network-demo", Name: "demo_default", Labels: map[string]string{"com.docker.compose.project": "demo"}}}, nil
		},
	}

	inventory, err := buildComposeDestroyInventory(context.Background(), client, composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: projectDir, RelativePath: "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Fingerprint == "" {
		t.Fatal("destroy inventory must include a fingerprint")
	}
	if got := destroyResourceNames(inventory.Delete, "container"); !reflect.DeepEqual(got, []string{"demo-db", "demo-web"}) {
		t.Fatalf("containers to delete = %#v", got)
	}
	if got := destroyResourceNames(inventory.Delete, "network"); !reflect.DeepEqual(got, []string{"demo_default"}) {
		t.Fatalf("networks to delete = %#v", got)
	}
	if got := destroyResourceNames(inventory.Delete, "volume"); !reflect.DeepEqual(got, []string{"demo_data"}) {
		t.Fatalf("volumes to delete = %#v", got)
	}
	if got := destroyResourceNames(inventory.Delete, "image"); !reflect.DeepEqual(got, []string{"example/web:latest"}) {
		t.Fatalf("images to delete = %#v", got)
	}
	if got := destroyResourceNames(inventory.Delete, "project_directory"); !reflect.DeepEqual(got, []string{projectDir}) {
		t.Fatalf("project directories to delete = %#v", got)
	}
	if got := destroyResourceNames(inventory.Retain, "bind"); !reflect.DeepEqual(got, []string{"/volume1/media"}) {
		t.Fatalf("binds to retain = %#v", got)
	}
	if got := destroyResourceNames(inventory.Retain, "volume"); !reflect.DeepEqual(got, []string{"shared_external"}) {
		t.Fatalf("volumes to retain = %#v", got)
	}
	if got := destroyResourceNames(inventory.Retain, "image"); !reflect.DeepEqual(got, []string{"postgres:16"}) {
		t.Fatalf("images to retain = %#v", got)
	}
}

func TestComposeDestroyInventoryIncludesBuildImagesWithoutProjectContainers(t *testing.T) {
	target := composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	}
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "other", ImageID: "sha256:shared", Labels: map[string]string{composeProjectLabel: "other"}}}, nil
		},
		ImageListFunc: func(_ context.Context, options types.ImageListOptions) ([]types.ImageSummary, error) {
			if !options.All || !reflect.DeepEqual(options.Filters.Get("label"), []string{composeProjectLabel + "=demo"}) {
				t.Fatalf("build images must be enumerated by exact project label: %#v", options)
			}
			return []types.ImageSummary{
				{ID: "sha256:build", RepoTags: []string{"demo-web:latest"}, Labels: map[string]string{composeProjectLabel: "demo"}},
				{ID: "sha256:dangling", RepoTags: []string{"<none>:<none>"}, Labels: map[string]string{composeProjectLabel: "demo"}},
				{ID: "sha256:shared", RepoTags: []string{"demo-db:latest"}, Labels: map[string]string{composeProjectLabel: "demo"}},
				{ID: "sha256:foreign", RepoTags: []string{"demo-lookalike:latest"}, Labels: map[string]string{composeProjectLabel: "other"}},
			}, nil
		},
	}
	inventory, err := buildComposeDestroyInventory(context.Background(), client, target)
	if err != nil {
		t.Fatal(err)
	}
	if !containsDestroyResource(inventory.Delete, "image", "demo-web:latest") ||
		!containsDestroyResource(inventory.Delete, "image", "dangling") {
		t.Fatalf("unused tagged/dangling project builds missing: %#v", inventory.Delete)
	}
	if !containsDestroyResource(inventory.Retain, "image", "demo-db:latest") ||
		containsDestroyResource(inventory.Delete, "image", "demo-lookalike:latest") {
		t.Fatalf("shared/foreign build images must not be deleted: %#v", inventory)
	}
	removed := []string{}
	client.ImageRemoveFunc = func(_ context.Context, id string, options types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
		if options.Force || options.PruneChildren {
			t.Fatalf("confirmed build deletion must preserve Docker reference checks: %#v", options)
		}
		removed = append(removed, id)
		return nil, nil
	}
	withComposeDestroyTestDependencies(t, client, func(composeOperationTarget) error {
		t.Fatal("selecting images must not delete project records or directory")
		return nil
	})
	selected := []string{}
	for _, resource := range inventory.Delete {
		if resource.Kind == "image" {
			selected = append(selected, resource.Key)
		}
	}
	_, err = executeComposeDestroy(context.Background(), target, inventory.Fingerprint, selected, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	if !reflect.DeepEqual(removed, []string{"sha256:build", "sha256:dangling"}) {
		t.Fatalf("deleted build images = %#v", removed)
	}
}

func TestComposeDestroyInventoryRejectsFailedBuildImageEnumeration(t *testing.T) {
	client := &tradisdocker.MockDockerClient{
		ImageListFunc: func(context.Context, types.ImageListOptions) ([]types.ImageSummary, error) {
			return nil, errors.New("daemon unavailable")
		},
	}
	_, err := buildComposeDestroyInventory(context.Background(), client, composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	})
	if err == nil || !strings.Contains(err.Error(), "daemon unavailable") {
		t.Fatalf("incomplete inventory must not be presented as complete: %v", err)
	}
}

func TestComposeDestroyInventoryFingerprintIncludesResourceIdentity(t *testing.T) {
	base := composeDestroyInventory{
		Project: "demo",
		Delete:  []composeDestroyResource{{Kind: "container", Name: "demo-web", ID: "container-a"}},
	}
	changed := base
	changed.Delete = []composeDestroyResource{{Kind: "container", Name: "demo-web", ID: "container-b"}}
	if composeDestroyInventoryFingerprint(base) == composeDestroyInventoryFingerprint(changed) {
		t.Fatal("resource identity change must invalidate the destroy confirmation fingerprint")
	}
}

func TestComposeDestroyInventoryFingerprintIgnoresDockerListOrder(t *testing.T) {
	target := composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	}
	containers := []types.Container{
		{ID: "one", Names: []string{"/demo-one"}, Image: "demo:z", ImageID: "sha256:same", Labels: map[string]string{composeProjectLabel: "demo"}, Mounts: []types.MountPoint{{Type: "volume", Name: "demo_data", Destination: "/z"}}},
		{ID: "two", Names: []string{"/demo-two"}, Image: "demo:a", ImageID: "sha256:same", Labels: map[string]string{composeProjectLabel: "demo"}, Mounts: []types.MountPoint{{Type: "volume", Name: "demo_data", Destination: "/a"}}},
	}
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) { return containers, nil },
		VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{Volumes: []*volume.Volume{{Name: "demo_data", Labels: map[string]string{composeProjectLabel: "demo"}}}}, nil
		},
		NetworkListFunc: func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) { return nil, nil },
	}
	first, err := buildComposeDestroyInventory(context.Background(), client, target)
	if err != nil {
		t.Fatal(err)
	}
	containers[0], containers[1] = containers[1], containers[0]
	second, err := buildComposeDestroyInventory(context.Background(), client, target)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatalf("same inventory must have stable fingerprint: %s != %s", first.Fingerprint, second.Fingerprint)
	}
}

func TestSelectComposeDestroyResourcesRejectsEmptyOrUnknownSelection(t *testing.T) {
	inventory := composeDestroyInventory{Delete: []composeDestroyResource{
		{Kind: "container", Name: "demo-web", ID: "container-web"},
	}}

	if _, err := selectComposeDestroyResources(inventory, []string{}); err == nil {
		t.Fatal("empty explicit selection must be rejected")
	}
	if _, err := selectComposeDestroyResources(inventory, []string{"container:unknown"}); err == nil {
		t.Fatal("unknown resource selection must be rejected")
	}
}

func TestExecuteComposeDestroyKeepsUncheckedContainerDependencies(t *testing.T) {
	target := composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	}
	projectContainers := []types.Container{
		{
			ID: "web", Names: []string{"/demo-web"}, Image: "demo:web", ImageID: "sha256:web",
			Labels: map[string]string{composeProjectLabel: "demo"},
			Mounts: []types.MountPoint{{Type: "volume", Name: "demo_web_data"}},
		},
		{
			ID: "db", Names: []string{"/demo-db"}, Image: "postgres:16", ImageID: "sha256:postgres",
			Labels: map[string]string{composeProjectLabel: "demo"},
			Mounts: []types.MountPoint{{Type: "volume", Name: "demo_db_data"}},
		},
	}
	removedContainers := make([]string, 0)
	removedVolumes := make([]string, 0)
	removedImages := make([]string, 0)
	cleanedPersistentState := false
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
			remaining := make([]types.Container, 0, len(projectContainers))
			for _, item := range projectContainers {
				if item.ID != "web" || !containsComposeDestroyString(removedContainers, "web") {
					remaining = append(remaining, item)
				}
			}
			return remaining, nil
		},
		VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{Volumes: []*volume.Volume{
				{Name: "demo_web_data", Labels: map[string]string{composeProjectLabel: "demo"}},
				{Name: "demo_db_data", Labels: map[string]string{composeProjectLabel: "demo"}},
			}}, nil
		},
		NetworkListFunc: func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) {
			return nil, nil
		},
		ContainerRemoveFunc: func(_ context.Context, id string, _ types.ContainerRemoveOptions) error {
			removedContainers = append(removedContainers, id)
			return nil
		},
		VolumeRemoveFunc: func(_ context.Context, id string, _ bool) error {
			removedVolumes = append(removedVolumes, id)
			return nil
		},
		ImageRemoveFunc: func(_ context.Context, id string, _ types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
			removedImages = append(removedImages, id)
			return nil, nil
		},
	}
	withComposeDestroyTestDependencies(t, client, func(composeOperationTarget) error {
		cleanedPersistentState = true
		return nil
	})

	inventory, err := buildComposeDestroyInventory(context.Background(), client, target)
	if err != nil {
		t.Fatal(err)
	}
	selected := []string{composeDestroyResourceKey(composeDestroyResource{Kind: "container", ID: "web"})}
	result, err := executeComposeDestroy(context.Background(), target, inventory.Fingerprint, selected, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removedContainers, []string{"web"}) {
		t.Fatalf("removed containers = %#v", removedContainers)
	}
	if len(removedVolumes) != 0 || len(removedImages) != 0 {
		t.Fatalf("unchecked container dependencies must remain, volumes=%#v images=%#v", removedVolumes, removedImages)
	}
	if cleanedPersistentState {
		t.Fatal("project directory and metadata must remain while an unchecked project container exists")
	}
	retained, _ := result["retained"].([]composeDestroyResource)
	if !containsDestroyResource(retained, "container", "demo-db") || !containsDestroyResource(retained, "project_directory", target.ProjectDir) {
		t.Fatalf("expected unchecked container and project state in retained list: %#v", retained)
	}
}

func TestExecuteComposeDestroyRejectsStaleInventoryBeforeCleanup(t *testing.T) {
	target := composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	}
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "container-new", Names: []string{"/demo-web"}, Image: "demo:latest", ImageID: "sha256:image-new", Labels: map[string]string{composeProjectLabel: "demo"}}}, nil
		},
		VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{}, nil
		},
		NetworkListFunc: func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) { return nil, nil },
	}
	client.ContainerRemoveFunc = func(context.Context, string, types.ContainerRemoveOptions) error {
		t.Fatal("stale inventory must be rejected before Docker cleanup")
		return nil
	}
	withComposeDestroyTestDependencies(t, client, func(composeOperationTarget) error {
		t.Fatal("stale inventory must be rejected before persistent state cleanup")
		return nil
	})

	_, err := executeComposeDestroy(context.Background(), target, "old-fingerprint", nil, func(string, string) {})
	if err == nil || !strings.Contains(err.Error(), "销毁清单已变化") {
		t.Fatalf("expected stale inventory error, got %v", err)
	}
}

func TestExecuteComposeDestroyDeletesOwnedVolumeAndUnsharedImage(t *testing.T) {
	target := composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	}
	listCalls := 0
	removedVolumes := make([]string, 0)
	removedImages := make([]string, 0)
	removedContainers := make([]string, 0)
	removedNetworks := make([]string, 0)
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
			listCalls++
			if listCalls == 1 {
				return []types.Container{{
					ID: "container-demo", Names: []string{"/demo-web"}, Image: "demo:latest", ImageID: "sha256:image-demo",
					Labels: map[string]string{composeProjectLabel: "demo"}, Mounts: []types.MountPoint{{Type: "volume", Name: "demo_data"}},
				}}, nil
			}
			return nil, nil
		},
		VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
			if len(removedVolumes) != 0 {
				return volume.ListResponse{}, nil
			}
			return volume.ListResponse{Volumes: []*volume.Volume{{Name: "demo_data", Labels: map[string]string{composeProjectLabel: "demo"}}}}, nil
		},
		NetworkListFunc: func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) {
			if len(removedNetworks) != 0 {
				return nil, nil
			}
			return []types.NetworkResource{{ID: "network-demo", Name: "demo_default", Labels: map[string]string{composeProjectLabel: "demo"}}}, nil
		},
		ContainerRemoveFunc: func(_ context.Context, id string, opts types.ContainerRemoveOptions) error {
			if !opts.Force || opts.RemoveVolumes {
				t.Fatalf("container remove options must not delete unlisted volumes: %#v", opts)
			}
			removedContainers = append(removedContainers, id)
			return nil
		},
		NetworkRemoveFunc: func(_ context.Context, id string) error {
			removedNetworks = append(removedNetworks, id)
			return nil
		},
		VolumeRemoveFunc: func(_ context.Context, name string, _ bool) error {
			removedVolumes = append(removedVolumes, name)
			return nil
		},
		ImageRemoveFunc: func(_ context.Context, id string, _ types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
			removedImages = append(removedImages, id)
			return nil, nil
		},
	}
	withComposeDestroyTestDependencies(t, client, func(composeOperationTarget) error { return nil })

	inventory, err := buildComposeDestroyInventory(context.Background(), client, target)
	if err != nil {
		t.Fatal(err)
	}
	listCalls = 0
	result, err := executeComposeDestroy(context.Background(), target, inventory.Fingerprint, nil, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(removedVolumes, []string{"demo_data"}) {
		t.Fatalf("removed volumes = %#v", removedVolumes)
	}
	if !reflect.DeepEqual(removedContainers, []string{"container-demo"}) {
		t.Fatalf("removed containers = %#v", removedContainers)
	}
	if !reflect.DeepEqual(removedNetworks, []string{"network-demo"}) {
		t.Fatalf("removed networks = %#v", removedNetworks)
	}
	if !reflect.DeepEqual(removedImages, []string{"sha256:image-demo"}) {
		t.Fatalf("removed images = %#v", removedImages)
	}
	if result["project"] != "demo" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestExecuteComposeDestroyRetainsImageThatBecomesShared(t *testing.T) {
	target := composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	}
	listCalls := 0
	imageRemoved := false
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
			listCalls++
			if listCalls <= 2 {
				return []types.Container{{ID: "demo", Names: []string{"/demo"}, Image: "demo:latest", ImageID: "sha256:shared-later", Labels: map[string]string{composeProjectLabel: "demo"}}}, nil
			}
			return []types.Container{{ID: "other", Names: []string{"/other"}, Image: "demo:latest", ImageID: "sha256:shared-later", Labels: map[string]string{composeProjectLabel: "other"}}}, nil
		},
		VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{}, nil
		},
		NetworkListFunc:     func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) { return nil, nil },
		ContainerRemoveFunc: func(context.Context, string, types.ContainerRemoveOptions) error { return nil },
		ImageRemoveFunc: func(context.Context, string, types.ImageRemoveOptions) ([]types.ImageDeleteResponseItem, error) {
			imageRemoved = true
			return nil, errors.New("shared image must not be removed")
		},
	}
	withComposeDestroyTestDependencies(t, client, func(composeOperationTarget) error { return nil })

	inventory, err := buildComposeDestroyInventory(context.Background(), client, target)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeComposeDestroy(context.Background(), target, inventory.Fingerprint, nil, func(string, string) {})
	if err != nil {
		t.Fatal(err)
	}
	if imageRemoved {
		t.Fatal("image used by another container during destroy must be retained")
	}
}

func TestExecuteComposeDestroyPreservesDirectoryWhenProjectContainerReappears(t *testing.T) {
	target := composeOperationTarget{
		DisplayName: "demo", ComposeProjectName: "demo", ProjectDir: filepath.Join(t.TempDir(), "demo"), RelativePath: "demo",
	}
	listCalls := 0
	client := &tradisdocker.MockDockerClient{
		ContainerListFunc: func(context.Context, types.ContainerListOptions) ([]types.Container, error) {
			listCalls++
			switch listCalls {
			case 1, 2:
				return []types.Container{{ID: "old", Names: []string{"/demo-web"}, Labels: map[string]string{composeProjectLabel: "demo"}}}, nil
			default:
				return []types.Container{{ID: "new", Names: []string{"/demo-web"}, Labels: map[string]string{composeProjectLabel: "demo"}}}, nil
			}
		},
		VolumeListFunc: func(context.Context, volume.ListOptions) (volume.ListResponse, error) {
			return volume.ListResponse{}, nil
		},
		NetworkListFunc:     func(context.Context, types.NetworkListOptions) ([]types.NetworkResource, error) { return nil, nil },
		ContainerRemoveFunc: func(context.Context, string, types.ContainerRemoveOptions) error { return nil },
	}
	withComposeDestroyTestDependencies(t, client, func(composeOperationTarget) error {
		t.Fatal("project directory must be retained when a new project container appears")
		return nil
	})
	inventory, err := buildComposeDestroyInventory(context.Background(), client, target)
	if err != nil {
		t.Fatal(err)
	}
	_, err = executeComposeDestroy(context.Background(), target, inventory.Fingerprint, nil, func(string, string) {})
	if err == nil || !strings.Contains(err.Error(), "出现或残留了项目容器") {
		t.Fatalf("expected residual project resource error, got %v", err)
	}
}

func withComposeDestroyTestDependencies(
	t *testing.T,
	client composeDestroyDockerClient,
	persistentCleaner func(composeOperationTarget) error,
) {
	t.Helper()
	previousFactory := composeDestroyDockerClientFactory
	previousPersistentCleaner := composeDestroyPersistentStateCleaner
	composeDestroyDockerClientFactory = func() (composeDestroyDockerClient, error) { return client, nil }
	composeDestroyPersistentStateCleaner = persistentCleaner
	t.Cleanup(func() {
		composeDestroyDockerClientFactory = previousFactory
		composeDestroyPersistentStateCleaner = previousPersistentCleaner
	})
}

func destroyResourceNames(items []composeDestroyResource, kind string) []string {
	result := make([]string, 0)
	for _, item := range items {
		if item.Kind == kind {
			result = append(result, item.Name)
		}
	}
	return result
}

func containsComposeDestroyString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func containsDestroyResource(items []composeDestroyResource, kind, name string) bool {
	for _, item := range items {
		if item.Kind == kind && item.Name == name {
			return true
		}
	}
	return false
}
