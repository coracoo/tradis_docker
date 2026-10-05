package deployment

import (
	"context"
	"errors"
	"strings"
)

type ExecutionProgress struct {
	Stage   string         `json:"stage"`
	Message string         `json:"message"`
	Payload map[string]any `json:"payload,omitempty"`
}

type ExecutionProgressFunc func(ExecutionProgress)

type ExecutionResult struct {
	ProjectName string         `json:"projectName"`
	OperationID string         `json:"operationId,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Manifest    *Manifest      `json:"-"`
}

type PlanExecutionAdapter interface {
	Execute(context.Context, Plan, ExecutionProgressFunc) (ExecutionResult, error)
}

func ExecutePlan(
	ctx context.Context,
	plan Plan,
	adapter PlanExecutionAdapter,
	report ExecutionProgressFunc,
) (ExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ExecutionResult{}, NewStructuredError(
			ErrorCodeCanceled,
			err,
			false,
			nil,
			map[string]any{"stage": "execute"},
		)
	}
	if adapter == nil {
		return ExecutionResult{}, NewStructuredError(
			ErrorCodeInvalidInput,
			errors.New("deployment execution adapter is required"),
			false,
			nil,
			map[string]any{"stage": "execute"},
		)
	}
	if plan.Preflight.HasBlocking() {
		return ExecutionResult{}, NewStructuredError(
			ErrorCodePreflightBlocked,
			errors.New("deployment plan contains blocking preflight issues"),
			false,
			[]string{"fix_preflight", "ask_user"},
			map[string]any{"issues": plan.Preflight.IssuesBySeverity(PreflightBlocking)},
		)
	}
	if err := validateExecutablePlan(plan); err != nil {
		return ExecutionResult{}, NewStructuredError(
			ErrorCodeInvalidInput,
			err,
			false,
			[]string{"rebuild_plan"},
			map[string]any{"project": strings.TrimSpace(plan.Intent.ProjectName)},
		)
	}

	result, err := adapter.Execute(ctx, plan, report)
	if err != nil {
		var structured *StructuredError
		if errors.As(err, &structured) {
			return result, err
		}
		return result, NewStructuredError(
			ErrorCodeExecutionFailed,
			err,
			true,
			[]string{"inspect_deployment", "retry"},
			map[string]any{"project": plan.Intent.ProjectName},
		)
	}
	if strings.TrimSpace(result.ProjectName) == "" {
		result.ProjectName = strings.TrimSpace(plan.Intent.ProjectName)
	}
	return result, nil
}

func validateExecutablePlan(plan Plan) error {
	if strings.TrimSpace(plan.Intent.ProjectName) == "" {
		return errors.New("deployment project name is required")
	}
	switch plan.Intent.Kind {
	case IntentKindCompose, IntentKindBuild:
		if strings.TrimSpace(plan.RuntimeCompose) == "" {
			return errors.New("runtime Compose is required")
		}
	case IntentKindImage:
		if strings.TrimSpace(plan.Intent.Image) == "" {
			return errors.New("image reference is required")
		}
	default:
		return errors.New("unsupported deployment intent kind")
	}
	return nil
}
