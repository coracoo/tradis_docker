package composehistory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FileUpdate struct {
	Path    string
	Content []byte
	Mode    fs.FileMode
}

var replaceFilesRename = os.Rename

type stagedFileUpdate struct {
	path         string
	temporary    string
	rollback     string
	content      []byte
	mode         fs.FileMode
	originalMode fs.FileMode
	existed      bool
	backedUp     bool
	installed    bool
}

func ReplaceFiles(updates []FileUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	staged, err := stageFileUpdates(updates)
	if err != nil {
		return err
	}
	defer cleanupStagedFiles(staged)

	if err := backupTargets(staged); err != nil {
		return rollbackFileUpdates(staged, err)
	}
	for _, item := range staged {
		if err := replaceFilesRename(item.temporary, item.path); err != nil {
			return rollbackFileUpdates(staged, fmt.Errorf("替换配置文件失败: %w", err))
		}
		item.temporary = ""
		item.installed = true
	}
	if err := syncFileDirectories(staged); err != nil {
		return rollbackFileUpdates(staged, fmt.Errorf("同步配置目录失败: %w", err))
	}

	for _, item := range staged {
		if item.rollback == "" {
			continue
		}
		if err := os.Remove(item.rollback); err == nil || os.IsNotExist(err) {
			item.rollback = ""
		}
	}
	_ = syncFileDirectories(staged)
	return nil
}

func stageFileUpdates(updates []FileUpdate) ([]*stagedFileUpdate, error) {
	seen := make(map[string]struct{}, len(updates))
	staged := make([]*stagedFileUpdate, 0, len(updates))
	cleanup := func() { cleanupStagedFiles(staged) }

	for _, update := range updates {
		path := filepath.Clean(strings.TrimSpace(update.Path))
		if path == "." || path == "" {
			cleanup()
			return nil, fmt.Errorf("配置文件路径不能为空")
		}
		if _, exists := seen[path]; exists {
			cleanup()
			return nil, fmt.Errorf("配置文件路径重复")
		}
		seen[path] = struct{}{}

		item := &stagedFileUpdate{path: path, content: update.Content, mode: update.Mode.Perm()}
		if item.mode == 0 {
			item.mode = 0644
		}
		info, err := os.Stat(path)
		switch {
		case err == nil:
			if !info.Mode().IsRegular() {
				cleanup()
				return nil, fmt.Errorf("配置目标不是普通文件")
			}
			item.existed = true
			item.originalMode = info.Mode().Perm()
			item.mode = item.originalMode
		case os.IsNotExist(err):
			item.existed = false
		default:
			cleanup()
			return nil, fmt.Errorf("读取配置文件状态失败: %w", err)
		}

		temporary, err := os.CreateTemp(filepath.Dir(path), ".tradis-config-*.tmp")
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("创建配置临时文件失败: %w", err)
		}
		item.temporary = temporary.Name()
		staged = append(staged, item)
		if err := writeStagedFile(temporary, item.content, item.mode); err != nil {
			cleanup()
			return nil, err
		}
	}
	return staged, nil
}

func writeStagedFile(file *os.File, content []byte, mode fs.FileMode) error {
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("设置配置临时文件权限失败: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("写入配置临时文件失败: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("同步配置临时文件失败: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭配置临时文件失败: %w", err)
	}
	closed = true
	return nil
}

func backupTargets(staged []*stagedFileUpdate) error {
	for _, item := range staged {
		if !item.existed {
			continue
		}
		rollbackFile, err := os.CreateTemp(filepath.Dir(item.path), ".tradis-rollback-*.bak")
		if err != nil {
			return fmt.Errorf("创建配置回滚文件失败: %w", err)
		}
		item.rollback = rollbackFile.Name()
		if err := rollbackFile.Close(); err != nil {
			return fmt.Errorf("关闭配置回滚文件失败: %w", err)
		}
		if err := os.Remove(item.rollback); err != nil {
			return fmt.Errorf("准备配置回滚路径失败: %w", err)
		}
		if err := replaceFilesRename(item.path, item.rollback); err != nil {
			return fmt.Errorf("备份当前配置文件失败: %w", err)
		}
		item.backedUp = true
	}
	return nil
}

func rollbackFileUpdates(staged []*stagedFileUpdate, cause error) error {
	var rollbackErrors []error
	for i := len(staged) - 1; i >= 0; i-- {
		item := staged[i]
		if item.installed {
			if err := os.Remove(item.path); err != nil && !os.IsNotExist(err) {
				rollbackErrors = append(rollbackErrors, err)
			}
			item.installed = false
		}
	}
	for i := len(staged) - 1; i >= 0; i-- {
		item := staged[i]
		if !item.backedUp || item.rollback == "" {
			continue
		}
		if err := replaceFilesRename(item.rollback, item.path); err != nil {
			rollbackErrors = append(rollbackErrors, err)
			continue
		}
		item.rollback = ""
		item.backedUp = false
	}
	if err := syncFileDirectories(staged); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
	if len(rollbackErrors) > 0 {
		return fmt.Errorf("%w；配置回滚失败: %v", cause, errors.Join(rollbackErrors...))
	}
	return cause
}

func syncFileDirectories(staged []*stagedFileUpdate) error {
	seen := map[string]struct{}{}
	for _, item := range staged {
		dir := filepath.Dir(item.path)
		if _, exists := seen[dir]; exists {
			continue
		}
		seen[dir] = struct{}{}
		handle, err := os.Open(dir)
		if err != nil {
			return err
		}
		syncErr := handle.Sync()
		closeErr := handle.Close()
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func cleanupStagedFiles(staged []*stagedFileUpdate) {
	for _, item := range staged {
		if item.temporary != "" {
			_ = os.Remove(item.temporary)
		}
		if item.rollback != "" {
			_ = os.Remove(item.rollback)
		}
	}
}
