package deployment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PathBoundaryError reports a candidate path that resolves outside its allowed
// root. Repository-controlled Compose paths (env_file, build context, Dockerfile,
// bind mounts, extra_files) must be validated so a malicious source cannot read,
// write, build from or mount outside an explicitly allowed root.
type PathBoundaryError struct {
	Root      string
	Candidate string
	Resolved  string
}

func (e *PathBoundaryError) Error() string {
	if e.Resolved != "" {
		return fmt.Sprintf("path %q resolves to %q outside allowed root %q", e.Candidate, e.Resolved, e.Root)
	}
	return fmt.Sprintf("path %q escapes allowed root %q", e.Candidate, e.Root)
}

func pathRelEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel)
}

// LexicalPathWithinRoot verifies, without touching the filesystem, that
// candidate resolves within root. Relative candidates are joined to root;
// absolute candidates must already be under root. It catches explicit parent
// traversal and absolute escapes. Pair with ResolveWithinRoot when symlink
// chains must also be evaluated.
func LexicalPathWithinRoot(root, candidate string) error {
	root = filepath.Clean(root)
	var target string
	if filepath.IsAbs(candidate) {
		target = filepath.Clean(candidate)
	} else {
		target = filepath.Clean(filepath.Join(root, candidate))
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return &PathBoundaryError{Root: root, Candidate: candidate}
	}
	if pathRelEscapes(rel) {
		return &PathBoundaryError{Root: root, Candidate: candidate, Resolved: target}
	}
	return nil
}

// ResolveWithinRoot resolves candidate against root and verifies the real path
// stays within root, evaluating symlinks. For not-yet-existing targets it
// resolves the nearest existing ancestor and re-appends the missing tail, so
// creating a path through a symlink chain that escapes root is still rejected.
func ResolveWithinRoot(root, candidate string) (string, error) {
	lexicalRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	realRoot := lexicalRoot
	if resolvedRoot, evalErr := filepath.EvalSymlinks(lexicalRoot); evalErr == nil {
		realRoot = resolvedRoot
	}
	var target string
	if filepath.IsAbs(candidate) {
		target = filepath.Clean(candidate)
	} else {
		target = filepath.Clean(filepath.Join(lexicalRoot, candidate))
	}
	if err := LexicalPathWithinRoot(lexicalRoot, target); err != nil {
		return target, err
	}
	resolved, err := evalSymlinksNearestExisting(target)
	if err != nil {
		return "", err
	}
	rel, relErr := filepath.Rel(realRoot, resolved)
	if relErr != nil || pathRelEscapes(rel) {
		return resolved, &PathBoundaryError{Root: lexicalRoot, Candidate: candidate, Resolved: resolved}
	}
	return resolved, nil
}

func evalSymlinksNearestExisting(path string) (string, error) {
	path = filepath.Clean(path)
	if _, err := os.Lstat(path); err == nil {
		if real, evalErr := filepath.EvalSymlinks(path); evalErr == nil {
			return real, nil
		}
	}
	dir := path
	tail := ""
	for {
		if _, err := os.Lstat(dir); err == nil {
			real, evalErr := filepath.EvalSymlinks(dir)
			if evalErr != nil {
				return "", evalErr
			}
			if tail == "" {
				return real, nil
			}
			return filepath.Clean(filepath.Join(real, tail)), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", &PathBoundaryError{Candidate: path}
		}
		if tail == "" {
			tail = filepath.Base(dir)
		} else {
			tail = filepath.Join(filepath.Base(dir), tail)
		}
		dir = parent
	}
}
