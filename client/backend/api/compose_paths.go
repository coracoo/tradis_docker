package api

import (
	"dockerpanel/backend/pkg/deployment"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveProjectDir 将 projectName 解析为 root 下的绝对项目目录。
// 它会对名字进行规范化、校验正则、禁止路径遍历，并确保最终目录位于 root 之下。
func resolveProjectDir(root, projectName string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("项目根目录未配置")
	}

	name, ok := validateComposeProjectName(projectName)
	if !ok {
		return "", fmt.Errorf("项目名不合法: %s", projectName)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("无法解析项目根目录: %w", err)
	}

	projectDir, err := filepath.Abs(filepath.Join(absRoot, name))
	if err != nil {
		return "", fmt.Errorf("无法解析项目目录: %w", err)
	}

	rel, err := filepath.Rel(absRoot, projectDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("项目目录逃逸出根目录: %s", projectDir)
	}
	if _, err := deployment.ResolveWithinRoot(absRoot, name); err != nil {
		return "", fmt.Errorf("项目目录真实路径逃逸出根目录: %w", err)
	}

	return projectDir, nil
}

// resolveExistingManagedProjectDir resolves an existing first-level project
// directory by its exact filesystem name. Unlike resolveProjectDir it does not
// lowercase the reference or apply Docker Compose project-name rules: a
// filesystem directory and a Docker Compose project identity are separate.
func resolveExistingManagedProjectDir(root, reference string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("项目根目录未配置")
	}
	if _, err := validateManagedProjectReference(reference); err != nil {
		return "", err
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("无法解析项目根目录: %w", err)
	}
	projectDir := filepath.Join(absRoot, reference)
	resolved, err := deployment.ResolveWithinRoot(absRoot, projectDir)
	if err != nil {
		return "", fmt.Errorf("项目目录真实路径逃逸出根目录: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("项目目录不存在: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("项目路径不是目录: %s", resolved)
	}
	return resolved, nil
}

// validateManagedProjectReference validates a first-level managed directory
// reference without applying Docker Compose project-name restrictions.
func validateManagedProjectReference(reference string) (string, error) {
	if reference == "" || reference != strings.TrimSpace(reference) || reference == "." || reference == ".." || strings.ContainsAny(reference, `/\`) {
		return "", fmt.Errorf("项目目录引用不合法")
	}
	return reference, nil
}

// validatePathWithinRoot 校验 candidate 解析后是否仍然位于 root 之下。
// candidate 可以是相对路径或绝对路径；相对路径会基于 root 进行解析。
func validatePathWithinRoot(root, candidate string) error {
	root = strings.TrimSpace(root)
	candidate = strings.TrimSpace(candidate)
	if root == "" {
		return fmt.Errorf("项目根目录未配置")
	}
	if candidate == "" {
		return fmt.Errorf("路径为空")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("无法解析项目根目录: %w", err)
	}

	candidatePath := candidate
	if !filepath.IsAbs(candidatePath) {
		candidatePath = filepath.Join(absRoot, candidatePath)
	}
	absCandidate, err := filepath.Abs(candidatePath)
	if err != nil {
		return fmt.Errorf("无法解析路径: %w", err)
	}

	rel, err := filepath.Rel(absRoot, absCandidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("路径逃逸出项目根目录: %s", candidate)
	}
	if _, err := deployment.ResolveWithinRoot(absRoot, absCandidate); err != nil {
		return fmt.Errorf("路径真实位置逃逸出项目根目录: %w", err)
	}

	return nil
}

// safeJoinProjectPath 安全地拼接项目目录下的子路径。
// 返回的绝对路径保证位于项目 root 之下。
func safeJoinProjectPath(root, projectName string, segments ...string) (string, error) {
	projectDir, err := resolveProjectDir(root, projectName)
	if err != nil {
		return "", err
	}

	parts := append([]string{projectDir}, segments...)
	candidate := filepath.Join(parts...)

	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("无法解析子路径: %w", err)
	}

	rel, err := filepath.Rel(projectDir, absCandidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("子路径逃逸出项目目录: %s", filepath.Join(segments...))
	}
	if _, err := deployment.ResolveWithinRoot(projectDir, absCandidate); err != nil {
		return "", fmt.Errorf("子路径真实位置逃逸出项目目录: %w", err)
	}

	return absCandidate, nil
}

// ensureProjectDirExists 确保项目目录已存在，若不存在则返回错误。
// 用于 start/stop/restart 等操作前校验。
func ensureProjectDirExists(root, projectName string) (string, error) {
	projectDir, err := resolveProjectDir(root, projectName)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(projectDir); err != nil || !info.IsDir() {
		if err == nil {
			return "", fmt.Errorf("项目路径不是目录: %s", projectDir)
		}
		return "", fmt.Errorf("项目目录不存在: %w", err)
	}
	return projectDir, nil
}

// resolveExistingProjectBaseDir 返回能访问到指定项目目录的项目根目录。
// 优先容器内项目根目录；当容器内嵌套挂载被 Docker 丢弃、目录不可达时，
// 回退到宿主机 PROJECT_ROOT 同路径别名挂载（docker-compose 会保留同路径别名以匹配宿主机路径）。
func resolveExistingProjectBaseDir(projectName string) (string, error) {
	containerRoot := getProjectsBaseDir()
	if _, err := existingComposeProjectDir(containerRoot, projectName); err == nil {
		return containerRoot, nil
	}
	hostRoot := strings.TrimSpace(effectiveHostProjectRoot())
	if hostRoot != "" {
		hostRoot = filepath.Clean(hostRoot)
		if filepath.Clean(containerRoot) != hostRoot {
			if _, err := existingComposeProjectDir(hostRoot, projectName); err == nil {
				return hostRoot, nil
			}
		}
	}
	return "", fmt.Errorf("项目目录不存在: %s", projectName)
}
