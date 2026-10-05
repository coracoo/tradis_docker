package deployment

import (
	"errors"
	"fmt"
	"strings"
)

type JobState string

const (
	JobStateQueued       JobState = "queued"
	JobStatePlanning     JobState = "planning"
	JobStateWaitingInput JobState = "waiting_input"
	JobStateExecuting    JobState = "executing"
	JobStateVerifying    JobState = "verifying"
	JobStateSuccess      JobState = "success"
	JobStateFailed       JobState = "failed"
	JobStateCanceled     JobState = "canceled"
)

var ErrInvalidJobStateTransition = errors.New("invalid deployment job state transition")

func NormalizeJobState(state string) JobState {
	normalized := strings.ToLower(strings.TrimSpace(state))
	if normalized == "cancelled" {
		normalized = string(JobStateCanceled)
	}
	return JobState(normalized)
}

func IsValidJobState(state JobState) bool {
	switch NormalizeJobState(string(state)) {
	case JobStateQueued,
		JobStatePlanning,
		JobStateWaitingInput,
		JobStateExecuting,
		JobStateVerifying,
		JobStateSuccess,
		JobStateFailed,
		JobStateCanceled:
		return true
	default:
		return false
	}
}

func IsTerminalJobState(state JobState) bool {
	switch NormalizeJobState(string(state)) {
	case JobStateSuccess, JobStateFailed, JobStateCanceled:
		return true
	default:
		return false
	}
}

func CanTransitionJobState(from JobState, to JobState) bool {
	from = NormalizeJobState(string(from))
	to = NormalizeJobState(string(to))
	if !IsValidJobState(from) || !IsValidJobState(to) {
		return false
	}
	if from == to {
		return true
	}
	if IsTerminalJobState(from) {
		return false
	}
	if to == JobStateFailed || to == JobStateCanceled {
		return true
	}

	switch from {
	case JobStateQueued:
		return to == JobStatePlanning
	case JobStatePlanning:
		return to == JobStateWaitingInput || to == JobStateExecuting
	case JobStateWaitingInput:
		return to == JobStatePlanning
	case JobStateExecuting:
		return to == JobStateWaitingInput || to == JobStateVerifying
	case JobStateVerifying:
		return to == JobStateSuccess
	default:
		return false
	}
}

func ValidateJobStateTransition(from JobState, to JobState) error {
	if CanTransitionJobState(from, to) {
		return nil
	}
	return fmt.Errorf("%w: %q -> %q", ErrInvalidJobStateTransition, from, to)
}
