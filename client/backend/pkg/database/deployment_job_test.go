package database

import (
	"dockerpanel/backend/pkg/deployment"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestCreateDeploymentJobIsIdempotent(t *testing.T) {
	setupEnvironmentTestDB(t)

	first, err := CreateDeploymentJob(DeploymentJobRecord{
		ID:             "job-first",
		Kind:           "compose_deploy",
		SourceType:     "github",
		SourceRef:      "example/project",
		ProjectName:    "example-project",
		IdempotencyKey: "idem-1",
		InputJSON:      `{"compose":"services"}`,
	})
	if err != nil {
		t.Fatalf("CreateDeploymentJob: %v", err)
	}
	if first.EnvironmentID != LocalEnvironmentID || first.Status != DeploymentJobStateQueued {
		t.Fatalf("unexpected defaults: %#v", first)
	}

	duplicate, err := CreateDeploymentJob(DeploymentJobRecord{
		ID:             "job-duplicate",
		EnvironmentID:  LocalEnvironmentID,
		Kind:           "compose_deploy",
		IdempotencyKey: "idem-1",
		InputJSON:      `{"compose":"different input must not replay"}`,
	})
	if err != nil {
		t.Fatalf("CreateDeploymentJob duplicate: %v", err)
	}
	if duplicate.ID != first.ID {
		t.Fatalf("expected existing job %q, got %q", first.ID, duplicate.ID)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM deployment_jobs WHERE idempotency_key = 'idem-1'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one job for idempotency key, got %d", count)
	}
}

func TestCreateDeploymentJobConcurrentIdempotency(t *testing.T) {
	setupEnvironmentTestDB(t)

	const count = 12
	jobs := make([]DeploymentJobRecord, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	wg.Add(count)
	for index := 0; index < count; index++ {
		index := index
		go func() {
			defer wg.Done()
			jobs[index], errs[index] = CreateDeploymentJob(DeploymentJobRecord{
				ID:             fmt.Sprintf("job-idempotent-%d", index),
				Kind:           "compose_deploy",
				IdempotencyKey: "idem-concurrent-create",
			})
		}()
	}
	wg.Wait()

	winnerID := ""
	for index, err := range errs {
		if err != nil {
			t.Fatalf("create %d failed: %v", index, err)
		}
		if winnerID == "" {
			winnerID = jobs[index].ID
		}
		if jobs[index].ID != winnerID {
			t.Fatalf("expected all callers to receive %q, got %#v", winnerID, jobs)
		}
	}

	var stored int
	if err := db.QueryRow(`SELECT COUNT(*) FROM deployment_jobs WHERE idempotency_key = 'idem-concurrent-create'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("expected one stored job, got %d", stored)
	}
}

func TestUpdateDeploymentJobStateRejectsInvalidTransitions(t *testing.T) {
	setupEnvironmentTestDB(t)
	_, err := CreateDeploymentJob(DeploymentJobRecord{
		ID:             "job-state",
		Kind:           "compose_deploy",
		IdempotencyKey: "idem-state",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := UpdateDeploymentJobState("job-state", DeploymentJobStateUpdate{Status: DeploymentJobStateExecuting}); err == nil {
		t.Fatal("expected queued -> executing to be rejected")
	}

	for _, state := range []string{
		DeploymentJobStatePlanning,
		DeploymentJobStateExecuting,
		DeploymentJobStateVerifying,
		DeploymentJobStateSuccess,
	} {
		update := DeploymentJobStateUpdate{Status: state}
		if state == DeploymentJobStateSuccess {
			update.Result = map[string]any{"project": "demo"}
		}
		if _, err := UpdateDeploymentJobState("job-state", update); err != nil {
			t.Fatalf("transition to %s failed: %v", state, err)
		}
	}

	completed, err := GetDeploymentJob("job-state")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != DeploymentJobStateSuccess || completed.ResultJSON != `{"project":"demo"}` || completed.FinishedAt == "" {
		t.Fatalf("unexpected completed job: %#v", completed)
	}
	if _, err := UpdateDeploymentJobState("job-state", DeploymentJobStateUpdate{Status: DeploymentJobStateExecuting}); err == nil {
		t.Fatal("expected terminal job transition to be rejected")
	}
}

func TestUpdateDeploymentJobStateAllowsInterruptedExecutionToWaitForInput(t *testing.T) {
	setupEnvironmentTestDB(t)
	_, err := CreateDeploymentJob(DeploymentJobRecord{
		ID:             "job-interrupted",
		Kind:           "compose_deploy",
		IdempotencyKey: "idem-interrupted",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{DeploymentJobStatePlanning, DeploymentJobStateExecuting, DeploymentJobStateWaitingInput} {
		if _, err := UpdateDeploymentJobState("job-interrupted", DeploymentJobStateUpdate{Status: state}); err != nil {
			t.Fatalf("transition to %s failed: %v", state, err)
		}
	}
}

func TestListRecoverableDeploymentJobsExcludesTerminalStates(t *testing.T) {
	setupEnvironmentTestDB(t)

	for index, state := range []string{
		DeploymentJobStateQueued,
		DeploymentJobStateWaitingInput,
		DeploymentJobStateFailed,
	} {
		id := fmt.Sprintf("recover-%d", index)
		_, err := CreateDeploymentJob(DeploymentJobRecord{
			ID:             id,
			Kind:           "compose_deploy",
			IdempotencyKey: "idem-" + id,
		})
		if err != nil {
			t.Fatal(err)
		}
		if state == DeploymentJobStateWaitingInput {
			if _, err := UpdateDeploymentJobState(id, DeploymentJobStateUpdate{Status: DeploymentJobStatePlanning}); err != nil {
				t.Fatal(err)
			}
		}
		if state != DeploymentJobStateQueued {
			if _, err := UpdateDeploymentJobState(id, DeploymentJobStateUpdate{Status: state, Error: "contract failure"}); err != nil {
				t.Fatal(err)
			}
		}
	}

	jobs, err := ListRecoverableDeploymentJobs(20)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected queued and waiting_input jobs, got %#v", jobs)
	}
	for _, job := range jobs {
		if job.Status == DeploymentJobStateFailed {
			t.Fatalf("terminal job returned as recoverable: %#v", job)
		}
	}
}

func TestAppendDeploymentJobStepConcurrentUniqueSeq(t *testing.T) {
	setupEnvironmentTestDB(t)
	_, err := CreateDeploymentJob(DeploymentJobRecord{
		ID:             "job-concurrent",
		Kind:           "compose_deploy",
		IdempotencyKey: "idem-concurrent",
	})
	if err != nil {
		t.Fatal(err)
	}

	const count = 20
	steps := make([]DeploymentJobStepRecord, count)
	errs := make([]error, count)
	var wg sync.WaitGroup
	wg.Add(count)
	for index := 0; index < count; index++ {
		index := index
		go func() {
			defer wg.Done()
			steps[index], errs[index] = AppendDeploymentJobStep("job-concurrent", DeploymentJobStepInput{
				Type:    "tool_result",
				Stage:   "deploy",
				Message: fmt.Sprintf("step-%d", index),
				Payload: map[string]any{"index": index},
			})
		}()
	}
	wg.Wait()

	seqs := make([]int, 0, count)
	for index, err := range errs {
		if err != nil {
			t.Fatalf("step %d failed: %v", index, err)
		}
		seqs = append(seqs, int(steps[index].Seq))
	}
	sort.Ints(seqs)
	for index, seq := range seqs {
		if seq != index+1 {
			t.Fatalf("expected continuous seq 1..%d, got %v", count, seqs)
		}
	}

	errorStep, err := AppendDeploymentJobStep("job-concurrent", DeploymentJobStepInput{
		Type:      "tool_error",
		Stage:     "verify",
		Message:   "health check failed",
		ErrorCode: "unhealthy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if errorStep.ErrorCode != "unhealthy" || errorStep.Seq != count+1 {
		t.Fatalf("unexpected structured error step: %#v", errorStep)
	}

	stored, err := GetDeploymentJobStepsAfter("job-concurrent", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != count+1 {
		t.Fatalf("expected %d stored steps, got %d", count+1, len(stored))
	}
}

func TestSaveDeploymentManifestRedactsSensitiveComposeValues(t *testing.T) {
	setupEnvironmentTestDB(t)
	_, err := CreateDeploymentJob(DeploymentJobRecord{
		ID:             "job-manifest",
		Kind:           "compose_deploy",
		IdempotencyKey: "idem-manifest",
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := SaveDeploymentManifest(deployment.Manifest{
		JobID:          "job-manifest",
		EnvironmentID:  LocalEnvironmentID,
		SourceCompose:  "services:\n  app:\n    image: example/app\n    environment:\n      API_TOKEN: super-secret\n",
		RuntimeCompose: "services:\n  app:\n    image: example/app\n    environment:\n      API_TOKEN: super-secret\n",
		Changes:        []deployment.PlanChange{{Type: "port", Target: "services.app.ports[0]", Before: 8080, After: 50000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.SourceCompose, "super-secret") || !strings.Contains(stored.SourceCompose, deployment.RedactedValue) {
		t.Fatalf("manifest compose was not redacted: %s", stored.SourceCompose)
	}
	var changes []deployment.PlanChange
	if err := json.Unmarshal([]byte(stored.ChangesJSON), &changes); err != nil || len(changes) != 1 {
		t.Fatalf("invalid stored changes: %s (%v)", stored.ChangesJSON, err)
	}
}

func TestDeploymentJobPersistenceRedactsStructuredStepAndResultValues(t *testing.T) {
	setupEnvironmentTestDB(t)
	job, err := CreateDeploymentJob(DeploymentJobRecord{
		ID:             "job-redacted-structured-values",
		Kind:           "compose_deploy",
		IdempotencyKey: "idem-redacted-structured-values",
	})
	if err != nil {
		t.Fatal(err)
	}
	step, err := AppendDeploymentJobStep(job.ID, DeploymentJobStepInput{
		Type:    "progress",
		Payload: map[string]any{"api_token": "step-secret", "project": "demo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(step.PayloadJSON, "step-secret") || !strings.Contains(step.PayloadJSON, deployment.RedactedValue) {
		t.Fatalf("Deployment Job step leaked a structured secret: %s", step.PayloadJSON)
	}

	for _, status := range []string{DeploymentJobStatePlanning, DeploymentJobStateExecuting, DeploymentJobStateFailed} {
		job, err = UpdateDeploymentJobState(job.ID, DeploymentJobStateUpdate{
			Status: status,
			Result: map[string]any{"password": "result-secret", "project": "demo"},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(job.ResultJSON, "result-secret") || !strings.Contains(job.ResultJSON, deployment.RedactedValue) {
		t.Fatalf("Deployment Job result leaked a structured secret: %s", job.ResultJSON)
	}
}
