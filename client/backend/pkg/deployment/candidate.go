package deployment

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DeploymentCandidateVersion = 1
	maxDeploymentFiles         = 64
	maxDeploymentFileBytes     = 1 << 20
)

// IsVisibleComposeProjectDirectory excludes hidden and internal directories
// from the managed Compose project list. Preflight and deployment staging
// directories intentionally contain temporary Compose files but are not
// user-managed projects.
func IsVisibleComposeProjectDirectory(name string) bool {
	return name != "" && !strings.HasPrefix(name, ".")
}

type DeploymentSourceType string

const (
	DeploymentSourceManualCompose DeploymentSourceType = "manual_compose"
	DeploymentSourceAppStore      DeploymentSourceType = "appstore"
)

// DeploymentFile is an additional regular text file that belongs to a
// deployment candidate. Compose and .env are represented by dedicated fields.
type DeploymentFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Mode    uint32 `json:"mode,omitempty"`
}

type DeploymentOptions struct {
	AutoStart bool `json:"auto_start"`
	Pull      bool `json:"pull"`
	Rebuild   bool `json:"rebuild"`
	Replace   bool `json:"replace"`
}

// DeploymentCandidate is the transport-neutral deployment input shared by a
// local adapter and the future remote Agent protocol.
type DeploymentCandidate struct {
	Version        int                  `json:"version"`
	EnvironmentID  string               `json:"environment_id"`
	ProjectName    string               `json:"project_name"`
	SourceType     DeploymentSourceType `json:"source_type"`
	ComposeYAML    string               `json:"compose_yaml"`
	Dotenv         string               `json:"dotenv,omitempty"`
	Files          []DeploymentFile     `json:"files,omitempty"`
	Options        DeploymentOptions    `json:"options"`
	CandidateHash  string               `json:"candidate_hash"`
	IdempotencyKey string               `json:"idempotency_key"`
}

var deploymentCandidateProjectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// NormalizeDeploymentCandidate validates the transport boundary, canonicalizes
// textual inputs and either assigns or verifies the candidate digest.
func NormalizeDeploymentCandidate(candidate DeploymentCandidate) (DeploymentCandidate, error) {
	candidate.Version = normalizeCandidateVersion(candidate.Version)
	candidate.EnvironmentID = strings.TrimSpace(candidate.EnvironmentID)
	candidate.ProjectName = strings.TrimSpace(candidate.ProjectName)
	candidate.ComposeYAML = normalizeCandidateText(candidate.ComposeYAML)
	candidate.Dotenv = normalizeCandidateText(candidate.Dotenv)
	candidate.IdempotencyKey = strings.TrimSpace(candidate.IdempotencyKey)

	if candidate.Version != DeploymentCandidateVersion {
		return DeploymentCandidate{}, candidateInputError("version is unsupported")
	}
	if candidate.EnvironmentID == "" {
		return DeploymentCandidate{}, candidateInputError("environment_id is required")
	}
	if !deploymentCandidateProjectName.MatchString(candidate.ProjectName) {
		return DeploymentCandidate{}, candidateInputError("project_name is invalid")
	}
	if candidate.SourceType != DeploymentSourceManualCompose && candidate.SourceType != DeploymentSourceAppStore {
		return DeploymentCandidate{}, candidateInputError("source_type is unsupported")
	}
	if !candidateHasServices(candidate.ComposeYAML) {
		return DeploymentCandidate{}, candidateInputError("compose_yaml must contain services")
	}

	files, err := normalizeDeploymentFiles(candidate.Files)
	if err != nil {
		return DeploymentCandidate{}, err
	}
	candidate.Files = files

	digest := HashDeploymentCandidate(candidate)
	provided := strings.TrimSpace(candidate.CandidateHash)
	if provided != "" {
		decoded, err := hex.DecodeString(provided)
		if err != nil || len(decoded) != sha256.Size {
			return DeploymentCandidate{}, candidateInputError("candidate hash is invalid")
		}
		expected, _ := hex.DecodeString(digest)
		if subtle.ConstantTimeCompare(decoded, expected) != 1 {
			return DeploymentCandidate{}, candidateInputError("candidate hash does not match content")
		}
	}
	candidate.CandidateHash = digest
	return candidate, nil
}

func normalizeCandidateVersion(version int) int {
	if version == 0 {
		return DeploymentCandidateVersion
	}
	return version
}

func candidateInputError(reason string) error {
	return fmt.Errorf("deployment candidate: %s", reason)
}

func candidateHasServices(composeYAML string) bool {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(composeYAML), &document); err != nil {
		return false
	}
	services, ok := document["services"].(map[string]any)
	return ok && len(services) > 0
}

func normalizeDeploymentFiles(files []DeploymentFile) ([]DeploymentFile, error) {
	if len(files) == 0 {
		return nil, nil
	}
	if len(files) > maxDeploymentFiles {
		return nil, candidateInputError("too many extra files")
	}
	normalized := make([]DeploymentFile, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		file.Path = normalizeCandidateFilePath(file.Path)
		if file.Path == "" {
			return nil, candidateInputError("extra file path is invalid")
		}
		if isReservedCandidateFile(file.Path) {
			return nil, candidateInputError("extra file duplicates compose or dotenv")
		}
		if _, exists := seen[file.Path]; exists {
			return nil, candidateInputError("extra file path is duplicated")
		}
		if len(file.Content) > maxDeploymentFileBytes {
			return nil, candidateInputError("extra file exceeds size limit")
		}
		if file.Mode == 0 {
			file.Mode = 0o644
		}
		if file.Mode != 0o600 && file.Mode != 0o644 {
			return nil, candidateInputError("extra file mode is unsupported")
		}
		file.Content = normalizeCandidateText(file.Content)
		seen[file.Path] = struct{}{}
		normalized = append(normalized, file)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path })
	return normalized, nil
}

func normalizeCandidateFilePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsRune(raw, '\x00') || strings.Contains(raw, "\\") || strings.HasPrefix(raw, "/") {
		return ""
	}
	cleaned := path.Clean(raw)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return ""
	}
	return cleaned
}

func isReservedCandidateFile(filePath string) bool {
	switch strings.ToLower(filePath) {
	case ".env", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	default:
		return false
	}
}

func normalizeCandidateText(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

// HashDeploymentCandidate returns a stable SHA-256 digest for the candidate
// payload. Call NormalizeDeploymentCandidate before using the value for an
// execution boundary; Hash is intentionally usable by tests and adapters.
func HashDeploymentCandidate(candidate DeploymentCandidate) string {
	candidate.Version = normalizeCandidateVersion(candidate.Version)
	candidate.EnvironmentID = strings.TrimSpace(candidate.EnvironmentID)
	candidate.ProjectName = strings.TrimSpace(candidate.ProjectName)
	candidate.ComposeYAML = normalizeCandidateText(candidate.ComposeYAML)
	candidate.Dotenv = normalizeCandidateText(candidate.Dotenv)
	candidate.IdempotencyKey = ""
	candidate.CandidateHash = ""
	files := make([]DeploymentFile, 0, len(candidate.Files))
	for _, file := range candidate.Files {
		file.Path = normalizeCandidateFilePath(file.Path)
		file.Content = normalizeCandidateText(file.Content)
		if file.Mode == 0 {
			file.Mode = 0o644
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	candidate.Files = files

	payload, err := json.Marshal(struct {
		Version       int                  `json:"version"`
		EnvironmentID string               `json:"environment_id"`
		ProjectName   string               `json:"project_name"`
		SourceType    DeploymentSourceType `json:"source_type"`
		ComposeYAML   string               `json:"compose_yaml"`
		Dotenv        string               `json:"dotenv"`
		Files         []DeploymentFile     `json:"files"`
		Options       DeploymentOptions    `json:"options"`
	}{
		Version:       candidate.Version,
		EnvironmentID: candidate.EnvironmentID,
		ProjectName:   candidate.ProjectName,
		SourceType:    candidate.SourceType,
		ComposeYAML:   candidate.ComposeYAML,
		Dotenv:        candidate.Dotenv,
		Files:         candidate.Files,
		Options:       candidate.Options,
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
