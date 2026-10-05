package deployment

import (
	"context"
	"errors"
	"testing"
)

type planExecutionAdapterStub struct {
	calls  int
	result ExecutionResult
	err    error
}

func (stub *planExecutionAdapterStub) Execute(_ context.Context, _ Plan, report ExecutionProgressFunc) (ExecutionResult, error) {
	stub.calls++
	if report != nil {
		report(ExecutionProgress{Stage: "compose", Message: "started"})
	}
	return stub.result, stub.err
}

func TestExecutePlanCallsAdapterExactlyOnce(t *testing.T) {
	adapter := &planExecutionAdapterStub{result: ExecutionResult{ProjectName: "demo", OperationID: "task-1"}}
	progress := 0
	result, err := ExecutePlan(context.Background(), Plan{
		Intent:         Intent{Kind: IntentKindCompose, ProjectName: "demo"},
		RuntimeCompose: "services:\n  app:\n    image: nginx\n",
	}, adapter, func(ExecutionProgress) { progress++ })
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 || progress != 1 || result.OperationID != "task-1" {
		t.Fatalf("unexpected execution: calls=%d progress=%d result=%#v", adapter.calls, progress, result)
	}
}

func TestExecutePlanStopsBeforeBlockedPreflight(t *testing.T) {
	adapter := &planExecutionAdapterStub{}
	_, err := ExecutePlan(context.Background(), Plan{
		Intent:         Intent{Kind: IntentKindCompose, ProjectName: "demo"},
		RuntimeCompose: "services:\n  app:\n    image: nginx\n",
		Preflight: PreflightResult{Issues: []PreflightIssue{{
			Code:     "required_env_missing",
			Severity: PreflightBlocking,
		}}},
	}, adapter, nil)
	var structured *StructuredError
	if !errors.As(err, &structured) || structured.Code != ErrorCodePreflightBlocked || adapter.calls != 0 {
		t.Fatalf("blocked plan must not execute: calls=%d err=%v", adapter.calls, err)
	}
}
