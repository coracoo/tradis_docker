package api

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dockerpanel/backend/pkg/database"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

var errComposeIdentityConflict = errors.New("Compose 项目身份冲突")
var observeComposeProjectNames = observedComposeProjectNamesForPath

type resolvedComposeIdentity struct {
	RelativePath       string
	ComposeProjectName string
	Source             string
}

type composeOperationTarget struct {
	DisplayName        string
	ProjectDir         string
	RelativePath       string
	ComposeProjectName string
	IdentitySource     string
}

func isExactComposeProjectName(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && composeNameRe.MatchString(value)
}

func normalizeComposeIdentityRelativePath(value string) string {
	value = norm.NFC.String(filepath.ToSlash(filepath.Clean(strings.TrimSpace(value))))
	return strings.TrimPrefix(value, "./")
}

func deriveGeneratedComposeProjectName(relativePath string, occupied map[string]struct{}) string {
	normalized := normalizeComposeIdentityRelativePath(relativePath)
	sum := sha256.Sum256([]byte(normalized))
	encoded := hex.EncodeToString(sum[:])
	for _, size := range []int{8, 12, 16, len(encoded)} {
		candidate := "compose-" + encoded[:size]
		if _, exists := occupied[candidate]; !exists {
			return candidate
		}
	}
	return "compose-" + encoded
}

func composeProjectNameFromDotenv(content []byte) string {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "COMPOSE_PROJECT_NAME" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if isExactComposeProjectName(value) {
			return value
		}
		return ""
	}
	return ""
}

func composeProjectNameFromYAML(content []byte) string {
	var root struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal(content, &root); err != nil {
		return ""
	}
	name := strings.TrimSpace(root.Name)
	if !isExactComposeProjectName(name) {
		return ""
	}
	return name
}

func composeIdentityOccupiedNames(extra map[string]struct{}) map[string]struct{} {
	occupied := make(map[string]struct{}, len(extra))
	for name := range extra {
		occupied[name] = struct{}{}
	}
	records, err := database.ListComposeProjectIdentities(database.LocalEnvironmentID)
	if err != nil {
		return occupied
	}
	for _, record := range records {
		occupied[record.ComposeProjectName] = struct{}{}
	}
	return occupied
}

func uniqueValidComposeProjectNames(values []string) []string {
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !isExactComposeProjectName(value) {
			continue
		}
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func managedComposeRelativePath(projectDir string) (string, error) {
	relativePath, ok := pathRelativeToRoot(getProjectsBaseDir(), projectDir)
	if !ok || relativePath == "." {
		return "", fmt.Errorf("Compose 项目路径不在受管目录内")
	}
	return normalizeComposeIdentityRelativePath(relativePath), nil
}

func resolveManagedComposeIdentity(projectDir string, observedNames []string, occupiedNames map[string]struct{}) (resolvedComposeIdentity, error) {
	relativePath, err := managedComposeRelativePath(projectDir)
	if err != nil {
		return resolvedComposeIdentity{}, err
	}
	observed := uniqueValidComposeProjectNames(observedNames)
	if len(observed) > 1 {
		return resolvedComposeIdentity{}, fmt.Errorf("%w: 路径 %s 对应多个 Docker 项目 %s", errComposeIdentityConflict, relativePath, strings.Join(observed, ", "))
	}

	stored, found, err := database.GetComposeProjectIdentity(database.LocalEnvironmentID, relativePath)
	if err != nil {
		return resolvedComposeIdentity{}, err
	}
	if found {
		if len(observed) == 1 && observed[0] != stored.ComposeProjectName {
			return resolvedComposeIdentity{}, fmt.Errorf("%w: 已保存 %s，Docker 观察到 %s", errComposeIdentityConflict, stored.ComposeProjectName, observed[0])
		}
		return resolvedComposeIdentity{
			RelativePath: relativePath, ComposeProjectName: stored.ComposeProjectName, Source: stored.Source,
		}, nil
	}

	name := ""
	source := ""
	if len(observed) == 1 {
		name, source = observed[0], "docker_label"
	}
	if name == "" {
		if content, readErr := os.ReadFile(filepath.Join(projectDir, ".env")); readErr == nil {
			name = composeProjectNameFromDotenv(content)
			if name != "" {
				source = "dotenv"
			}
		}
	}
	if name == "" {
		composePath, findErr := findComposeFile(projectDir)
		if findErr == nil {
			if content, readErr := os.ReadFile(composePath); readErr == nil {
				name = composeProjectNameFromYAML(content)
				if name != "" {
					source = "yaml"
				}
			}
		}
	}
	if name == "" {
		directoryName := strings.ToLower(filepath.Base(filepath.Clean(projectDir)))
		if isExactComposeProjectName(directoryName) {
			name, source = directoryName, "directory"
		}
	}
	if name == "" {
		name = deriveGeneratedComposeProjectName(relativePath, composeIdentityOccupiedNames(occupiedNames))
		source = "generated"
	}

	record := database.ComposeProjectIdentity{
		EnvironmentID: database.LocalEnvironmentID, RelativePath: relativePath, ComposeProjectName: name, Source: source,
	}
	if err := database.UpsertComposeProjectIdentity(record); err != nil {
		return resolvedComposeIdentity{}, err
	}
	return resolvedComposeIdentity{RelativePath: relativePath, ComposeProjectName: name, Source: source}, nil
}

func observedComposeProjectNamesForPath(ctx context.Context, projectDir string) ([]string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	defer cli.Close()
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project")),
	})
	if err != nil {
		return nil, err
	}
	wanted := filepath.Clean(projectDir)
	observed := make([]string, 0)
	for _, container := range containers {
		labels := container.Labels
		projectName := strings.TrimSpace(labels["com.docker.compose.project"])
		workingDir := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
		configFiles := strings.TrimSpace(labels["com.docker.compose.project.config_files"])
		rawPath := workingDir
		if rawPath == "" && configFiles != "" {
			rawPath = filepath.Dir(strings.Split(configFiles, ",")[0])
		}
		if filepath.Clean(canonicalComposeObservedProjectPath(rawPath, projectName)) == wanted {
			observed = append(observed, projectName)
		}
	}
	return uniqueValidComposeProjectNames(observed), nil
}

func composeOperationTargetFromDirectory(ctx context.Context, projectDir string) (composeOperationTarget, error) {
	// Docker may be temporarily unavailable. File operations can still use
	// persisted/config evidence, but available runtime evidence is checked on
	// every resolution so stale mappings cannot target another stack.
	observed, _ := observeComposeProjectNames(ctx, projectDir)
	identity, err := resolveManagedComposeIdentity(projectDir, observed, nil)
	if err != nil {
		return composeOperationTarget{}, err
	}
	return composeOperationTarget{
		DisplayName:        filepath.Base(projectDir),
		ProjectDir:         projectDir,
		RelativePath:       identity.RelativePath,
		ComposeProjectName: identity.ComposeProjectName,
		IdentitySource:     identity.Source,
	}, nil
}

func resolveComposeOperationTarget(ctx context.Context, reference string) (composeOperationTarget, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" || reference == "." || reference == ".." || strings.ContainsAny(reference, `/\`) {
		return composeOperationTarget{}, fmt.Errorf("Compose 项目引用不合法")
	}
	root := getProjectsBaseDir()
	if projectDir, err := resolveExistingManagedProjectDir(root, reference); err == nil {
		return composeOperationTargetFromDirectory(ctx, projectDir)
	}

	records, err := database.ListComposeProjectIdentities(database.LocalEnvironmentID)
	if err != nil {
		return composeOperationTarget{}, err
	}
	for _, record := range records {
		if record.ComposeProjectName != reference {
			continue
		}
		projectDir, resolveErr := resolveExistingManagedProjectDir(root, record.RelativePath)
		if resolveErr != nil {
			return composeOperationTarget{}, resolveErr
		}
		return composeOperationTargetFromDirectory(ctx, projectDir)
	}

	name, ok := validateComposeProjectName(reference)
	if !ok {
		return composeOperationTarget{}, fmt.Errorf("Compose 项目不存在: %s", reference)
	}
	projectDir, err := existingComposeProjectDir(root, name)
	if err != nil {
		return composeOperationTarget{}, err
	}
	return composeOperationTargetFromDirectory(ctx, projectDir)
}

// resolveComposeRestoreTarget also resolves a deleted legacy project directory.
// The persisted identity keeps Unicode display paths tied to the original
// Compose project name; legacy legal names retain their former restore path.
func resolveComposeRestoreTarget(ctx context.Context, reference string) (composeOperationTarget, error) {
	return resolveComposeRestoreTargetWithName(ctx, reference, "")
}

func resolveComposeRestoreTargetWithName(ctx context.Context, reference, hintedComposeProjectName string) (composeOperationTarget, error) {
	if target, err := resolveComposeOperationTarget(ctx, reference); err == nil {
		return target, nil
	}
	reference = strings.TrimSpace(reference)
	if reference == "" || reference == "." || reference == ".." || strings.ContainsAny(reference, `/\`) {
		return composeOperationTarget{}, fmt.Errorf("Compose 项目引用不合法")
	}

	relativePath := normalizeComposeIdentityRelativePath(reference)
	composeProjectName := ""
	source := ""
	if record, found, err := database.GetComposeProjectIdentity(database.LocalEnvironmentID, relativePath); err != nil {
		return composeOperationTarget{}, err
	} else if found {
		composeProjectName = record.ComposeProjectName
		source = record.Source
	}
	if composeProjectName == "" && isExactComposeProjectName(hintedComposeProjectName) {
		composeProjectName = strings.TrimSpace(hintedComposeProjectName)
		source = "backup_manifest"
	}
	if composeProjectName == "" {
		name, ok := validateComposeProjectName(reference)
		if !ok {
			return composeOperationTarget{}, fmt.Errorf("Compose 项目身份不存在: %s", reference)
		}
		composeProjectName = name
		source = "directory"
	}

	root := getProjectsBaseDir()
	projectDir := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := validatePathWithinRoot(root, projectDir); err != nil {
		return composeOperationTarget{}, err
	}
	return composeOperationTarget{
		DisplayName: filepath.Base(projectDir), ProjectDir: projectDir, RelativePath: relativePath,
		ComposeProjectName: composeProjectName, IdentitySource: source,
	}, nil
}

func persistRestoredComposeIdentity(target composeOperationTarget) error {
	if target.RelativePath == "" || !isExactComposeProjectName(target.ComposeProjectName) {
		return fmt.Errorf("恢复后的 Compose 项目身份不完整")
	}
	source := target.IdentitySource
	if source == "" || source == "backup_manifest" {
		source = "restore_manifest"
	}
	return database.UpsertComposeProjectIdentity(database.ComposeProjectIdentity{
		EnvironmentID:      database.LocalEnvironmentID,
		RelativePath:       target.RelativePath,
		ComposeProjectName: target.ComposeProjectName,
		Source:             source,
	})
}

func composeCommandWithProjectName(projectName string, args []string) []string {
	if len(args) == 0 || args[0] != "compose" || !isExactComposeProjectName(projectName) {
		return append([]string(nil), args...)
	}
	result := make([]string, 0, len(args)+2)
	result = append(result, "compose", "--project-name", projectName)
	result = append(result, args[1:]...)
	return result
}
