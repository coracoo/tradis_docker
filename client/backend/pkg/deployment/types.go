package deployment

import (
	"fmt"
	"strings"
)

type SourceType string

const (
	SourceTypeGitHub   SourceType = "github"
	SourceTypeTutorial SourceType = "tutorial"
	SourceTypePrompt   SourceType = "prompt"
	SourceTypeCompose  SourceType = "compose"
	SourceTypeAppStore SourceType = "appstore"
)

type Source struct {
	Type     SourceType     `json:"type"`
	Ref      string         `json:"ref,omitempty"`
	RepoID   int64          `json:"repoId,omitempty"`
	PathHint string         `json:"pathHint,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type IntentKind string

const (
	IntentKindCompose IntentKind = "compose"
	IntentKindImage   IntentKind = "image"
	IntentKindBuild   IntentKind = "build"
)

type Intent struct {
	EnvironmentID  string         `json:"environmentId"`
	ProjectName    string         `json:"projectName"`
	Kind           IntentKind     `json:"kind"`
	Source         Source         `json:"source"`
	ComposeYAML    string         `json:"composeYaml,omitempty"`
	Image          string         `json:"image,omitempty"`
	BuildContext   string         `json:"buildContext,omitempty"`
	DockerfilePath string         `json:"dockerfilePath,omitempty"`
	Parameters     map[string]any `json:"parameters,omitempty"`
	Options        map[string]any `json:"options,omitempty"`
}

type Plan struct {
	Intent               Intent          `json:"intent"`
	RuntimeCompose       string          `json:"runtimeCompose,omitempty"`
	Changes              []PlanChange    `json:"changes,omitempty"`
	Preflight            PreflightResult `json:"preflight"`
	RequiresConfirmation bool            `json:"requiresConfirmation,omitempty"`
	Metadata             map[string]any  `json:"metadata,omitempty"`
	// Workspace 是 AI 工作区计划的候选身份；revision/candidateHash 参与
	// Deployment Job 幂等键，使同一可见参数下的新候选成为新尝试。普通
	// Compose/AppStore 计划不设置，幂等输入保持历史字段不变。
	Workspace *WorkspaceIdentity `json:"workspace,omitempty"`
}

type PlanChange struct {
	Type       string `json:"type"`
	Target     string `json:"target"`
	Before     any    `json:"before,omitempty"`
	After      any    `json:"after,omitempty"`
	ReasonCode string `json:"reasonCode,omitempty"`
}

type PreflightSeverity string

const (
	PreflightBlocking PreflightSeverity = "blocking"
	PreflightWarning  PreflightSeverity = "warning"
	PreflightInfo     PreflightSeverity = "info"
)

type PreflightIssue struct {
	Code        string            `json:"code"`
	Severity    PreflightSeverity `json:"severity"`
	Retryable   bool              `json:"retryable,omitempty"`
	NextActions []string          `json:"next_actions,omitempty"`
	Details     map[string]any    `json:"details,omitempty"`
}

type PreflightResult struct {
	Issues []PreflightIssue `json:"issues"`
}

func (result PreflightResult) HasBlocking() bool {
	return len(result.IssuesBySeverity(PreflightBlocking)) > 0
}

func (result PreflightResult) IssuesBySeverity(severity PreflightSeverity) []PreflightIssue {
	out := make([]PreflightIssue, 0)
	for _, issue := range result.Issues {
		if issue.Severity == severity {
			out = append(out, issue)
		}
	}
	return out
}

type ErrorCode string

const (
	ErrorCodeInvalidInput           ErrorCode = "invalid_input"
	ErrorCodeInvalidStateTransition ErrorCode = "invalid_state_transition"
	ErrorCodeSourceUnavailable      ErrorCode = "source_unavailable"
	ErrorCodePreflightBlocked       ErrorCode = "preflight_blocked"
	ErrorCodeExecutionFailed        ErrorCode = "execution_failed"
	ErrorCodeProjectExists          ErrorCode = "project_exists"
	ErrorCodeVerificationFailed     ErrorCode = "verification_failed"
	ErrorCodeCanceled               ErrorCode = "canceled"
)

type StructuredError struct {
	Code        ErrorCode      `json:"error_code"`
	Retryable   bool           `json:"retryable"`
	NextActions []string       `json:"next_actions,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
	cause       error
}

func NewStructuredError(
	code ErrorCode,
	cause error,
	retryable bool,
	nextActions []string,
	details map[string]any,
) *StructuredError {
	return &StructuredError{
		Code:        ErrorCode(strings.TrimSpace(string(code))),
		Retryable:   retryable,
		NextActions: normalizeNextActions(nextActions),
		Details:     redactMap(details),
		cause:       cause,
	}
}

func (err *StructuredError) Error() string {
	if err == nil {
		return ""
	}
	message := err.PublicMessage()
	if err.Code == "" {
		return message
	}
	return fmt.Sprintf("%s: %s", err.Code, message)
}

func (err *StructuredError) PublicMessage() string {
	if err == nil {
		return ""
	}
	switch err.Code {
	case ErrorCodeInvalidInput:
		return "部署输入无效"
	case ErrorCodeInvalidStateTransition:
		return "部署任务状态转换无效"
	case ErrorCodeSourceUnavailable:
		return "无法读取部署来源"
	case ErrorCodePreflightBlocked:
		return "部署预检未通过"
	case ErrorCodeExecutionFailed:
		return "部署执行失败"
	case ErrorCodeVerificationFailed:
		return "部署验证失败"
	case ErrorCodeCanceled:
		return "部署已取消"
	case ErrorCodeProjectExists:
		return "Compose 项目已存在，默认不会覆盖"
	default:
		return "部署操作失败"
	}
}

func (err *StructuredError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func normalizeNextActions(actions []string) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		if _, exists := seen[action]; exists {
			continue
		}
		seen[action] = struct{}{}
		out = append(out, action)
	}
	return out
}

var _ error = (*StructuredError)(nil)
