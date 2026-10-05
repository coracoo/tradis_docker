package deployment

import (
	"errors"
	"testing"
)

func TestJobStateTransitionMatrix(t *testing.T) {
	valid := []struct {
		from JobState
		to   JobState
	}{
		{JobStateQueued, JobStateQueued},
		{JobStateQueued, JobStatePlanning},
		{JobStatePlanning, JobStateWaitingInput},
		{JobStatePlanning, JobStateExecuting},
		{JobStateWaitingInput, JobStatePlanning},
		{JobStateExecuting, JobStateWaitingInput},
		{JobStateExecuting, JobStateVerifying},
		{JobStateVerifying, JobStateSuccess},
		{JobStateQueued, JobStateFailed},
		{JobStatePlanning, JobStateCanceled},
		{JobStateWaitingInput, JobStateFailed},
		{JobStateExecuting, JobStateCanceled},
		{JobStateVerifying, JobStateFailed},
	}

	for _, transition := range valid {
		if !CanTransitionJobState(transition.from, transition.to) {
			t.Errorf("expected transition %q -> %q to be valid", transition.from, transition.to)
		}
		if err := ValidateJobStateTransition(transition.from, transition.to); err != nil {
			t.Errorf("validate transition %q -> %q: %v", transition.from, transition.to, err)
		}
	}

	invalid := []struct {
		from JobState
		to   JobState
	}{
		{JobStateQueued, JobStateExecuting},
		{JobStatePlanning, JobStateSuccess},
		{JobStateWaitingInput, JobStateExecuting},
		{JobStateExecuting, JobStateSuccess},
		{JobStateVerifying, JobStatePlanning},
		{JobStateSuccess, JobStateExecuting},
		{JobStateFailed, JobStatePlanning},
		{JobStateCanceled, JobStateQueued},
		{JobState("unknown"), JobStateQueued},
		{JobStateQueued, JobState("unknown")},
	}

	for _, transition := range invalid {
		if CanTransitionJobState(transition.from, transition.to) {
			t.Errorf("expected transition %q -> %q to be invalid", transition.from, transition.to)
		}
		if err := ValidateJobStateTransition(transition.from, transition.to); !errors.Is(err, ErrInvalidJobStateTransition) {
			t.Errorf("expected transition error for %q -> %q, got %v", transition.from, transition.to, err)
		}
	}
}

func TestJobStateClassification(t *testing.T) {
	all := []JobState{
		JobStateQueued,
		JobStatePlanning,
		JobStateWaitingInput,
		JobStateExecuting,
		JobStateVerifying,
		JobStateSuccess,
		JobStateFailed,
		JobStateCanceled,
	}
	for _, state := range all {
		if !IsValidJobState(state) {
			t.Errorf("expected %q to be valid", state)
		}
	}

	for _, state := range []JobState{JobStateSuccess, JobStateFailed, JobStateCanceled} {
		if !IsTerminalJobState(state) {
			t.Errorf("expected %q to be terminal", state)
		}
	}
	if IsTerminalJobState(JobStateVerifying) {
		t.Fatal("verifying must remain recoverable")
	}

	if got := NormalizeJobState("  CANCELLED  "); got != JobStateCanceled {
		t.Fatalf("expected cancelled alias to normalize to %q, got %q", JobStateCanceled, got)
	}
}

func TestJobStateTransitionMatrixIsExhaustive(t *testing.T) {
	states := []JobState{
		JobStateQueued,
		JobStatePlanning,
		JobStateWaitingInput,
		JobStateExecuting,
		JobStateVerifying,
		JobStateSuccess,
		JobStateFailed,
		JobStateCanceled,
	}
	expected := map[JobState]map[JobState]bool{
		JobStateQueued: {
			JobStateQueued: true, JobStatePlanning: true, JobStateFailed: true, JobStateCanceled: true,
		},
		JobStatePlanning: {
			JobStatePlanning: true, JobStateWaitingInput: true, JobStateExecuting: true, JobStateFailed: true, JobStateCanceled: true,
		},
		JobStateWaitingInput: {
			JobStateWaitingInput: true, JobStatePlanning: true, JobStateFailed: true, JobStateCanceled: true,
		},
		JobStateExecuting: {
			JobStateExecuting: true, JobStateWaitingInput: true, JobStateVerifying: true, JobStateFailed: true, JobStateCanceled: true,
		},
		JobStateVerifying: {
			JobStateVerifying: true, JobStateSuccess: true, JobStateFailed: true, JobStateCanceled: true,
		},
		JobStateSuccess:  {JobStateSuccess: true},
		JobStateFailed:   {JobStateFailed: true},
		JobStateCanceled: {JobStateCanceled: true},
	}

	for _, from := range states {
		for _, to := range states {
			if got := CanTransitionJobState(from, to); got != expected[from][to] {
				t.Errorf("transition %q -> %q: got %t, want %t", from, to, got, expected[from][to])
			}
		}
	}
}
