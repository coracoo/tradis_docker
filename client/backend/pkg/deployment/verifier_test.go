package deployment

import (
	"context"
	"errors"
	"testing"
	"time"
)

type verificationObserverStub struct {
	items [][]ContainerObservation
	calls int
}

func (stub *verificationObserverStub) ObserveDeployment(context.Context, string, string) ([]ContainerObservation, error) {
	stub.calls++
	if len(stub.items) == 0 {
		return nil, nil
	}
	index := stub.calls - 1
	if index >= len(stub.items) {
		index = len(stub.items) - 1
	}
	return stub.items[index], nil
}

func TestVerifyDeploymentAcceptsHealthyAndNoHealthcheckContainers(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{{
		{ID: "one", Name: "healthy", State: "running", Health: "healthy"},
		{ID: "two", Name: "plain", State: "running"},
	}}}

	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || len(result.Containers) != 2 || result.Attempts != 1 {
		t.Fatalf("unexpected verification result: %#v", result)
	}
}

func TestVerifyDeploymentWaitsForStartingHealthcheck(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{
		{{ID: "one", Name: "app", State: "running", Health: "starting"}},
		{{ID: "one", Name: "app", State: "running", Health: "healthy"}},
	}}

	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      2,
		Interval:      time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.Attempts != 2 || observer.calls != 2 {
		t.Fatalf("expected second observation to verify deployment: %#v", result)
	}
}

func TestVerifyDeploymentSupportsAdaptiveAttemptsBeyondLegacyCap(t *testing.T) {
	items := make([][]ContainerObservation, 0, 33)
	for index := 0; index < 32; index++ {
		items = append(items, []ContainerObservation{{ID: "one", Name: "app", State: "running", Health: "starting"}})
	}
	items = append(items, []ContainerObservation{{ID: "one", Name: "app", State: "running", Health: "healthy"}})
	observer := &verificationObserverStub{items: items}

	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      33,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.Attempts != 33 || observer.calls != 33 {
		t.Fatalf("adaptive verification was truncated by the legacy cap: result=%#v calls=%d", result, observer.calls)
	}
}

func TestVerifyDeploymentExtendsWindowFromObservedImageHealthcheck(t *testing.T) {
	items := make([][]ContainerObservation, 0, 33)
	for index := 0; index < 32; index++ {
		items = append(items, []ContainerObservation{{
			ID: "one", Name: "app", State: "running", Health: "starting",
			Healthcheck: &ContainerHealthcheck{
				Interval: 40 * time.Millisecond,
				Timeout:  5 * time.Millisecond,
				Retries:  1,
			},
		}})
	}
	items = append(items, []ContainerObservation{{ID: "one", Name: "app", State: "running", Health: "healthy"}})
	observer := &verificationObserverStub{items: items}

	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      15,
		Interval:      time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.Attempts != 33 || observer.calls != 33 {
		t.Fatalf("image healthcheck did not extend verification: result=%#v calls=%d", result, observer.calls)
	}
}

func TestVerifyDeploymentRejectsUnhealthyOrExitedContainers(t *testing.T) {
	tests := []struct {
		name string
		item ContainerObservation
		code string
	}{
		{name: "unhealthy", item: ContainerObservation{ID: "one", Name: "app", State: "running", Health: "unhealthy"}, code: "container_unhealthy"},
		{name: "exited", item: ContainerObservation{ID: "one", Name: "app", State: "exited"}, code: "container_not_running"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observer := &verificationObserverStub{items: [][]ContainerObservation{{test.item}}}
			result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
				EnvironmentID: "local",
				ProjectName:   "demo",
				Attempts:      1,
			})
			var structured *StructuredError
			if !errors.As(err, &structured) || structured.Code != ErrorCodeVerificationFailed {
				t.Fatalf("expected structured verification failure, got %v", err)
			}
			if result.Verified || len(result.Issues) == 0 || result.Issues[0].Code != test.code {
				t.Fatalf("unexpected verification failure: %#v", result)
			}
		})
	}
}

func TestVerifyDeploymentStopsImmediatelyOnTerminalContainerFailure(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{{{
		ID: "one", Name: "app", State: "running", Health: "unhealthy",
	}}}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      100,
	})
	if err == nil || result.Verified {
		t.Fatalf("terminal health failure must fail verification: result=%#v err=%v", result, err)
	}
	if observer.calls != 1 || result.Attempts != 1 {
		t.Fatalf("terminal health failure was polled unnecessarily: calls=%d result=%#v", observer.calls, result)
	}
}

func TestVerifyDeploymentProbesPublishedTCPPorts(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{{{
		ID:             "one",
		Name:           "app",
		State:          "running",
		Health:         "healthy",
		PublishedPorts: []PublishedPort{{HostIP: "0.0.0.0", HostPort: 50000, Protocol: "tcp"}},
	}}}}
	prober := PortProbeFunc(func(_ context.Context, host string, port int) error {
		if host != "127.0.0.1" || port != 50000 {
			t.Fatalf("unexpected probe target %s:%d", host, port)
		}
		return errors.New("connection refused")
	})

	result, err := VerifyDeployment(context.Background(), observer, prober, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      1,
		ProbeTCPPorts: true,
	})
	if err == nil || result.Verified || len(result.Issues) == 0 || result.Issues[0].Code != "port_unreachable" {
		t.Fatalf("expected port probe failure, result=%#v err=%v", result, err)
	}
}

func TestVerifyDeploymentRejectsEmptyProject(t *testing.T) {
	result, err := VerifyDeployment(context.Background(), &verificationObserverStub{}, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      1,
	})
	if err == nil || result.Verified || len(result.Issues) == 0 || result.Issues[0].Code != "containers_not_found" {
		t.Fatalf("zero containers must not be successful: result=%#v err=%v", result, err)
	}
}

func TestVerifyDeploymentRejectsContainerPredatingRecoveryBoundary(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{{{
		ID: "old", Name: "app", State: "running", Health: "healthy", CreatedAtUnix: 100,
	}}}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      1,
		NotBeforeUnix: 200,
	})
	if err == nil || result.Verified || len(result.Issues) == 0 || result.Issues[0].Code != "container_predates_job" {
		t.Fatalf("old same-project container must not verify a recovered deployment: result=%#v err=%v", result, err)
	}
}

func TestVerifyDeploymentAcceptsSuccessfullyCompletedInitContainers(t *testing.T) {
	zero, failing := 0, 1
	observer := &verificationObserverStub{items: [][]ContainerObservation{{
		{ID: "c1", Name: "web", State: "running", Health: "healthy"},
		{ID: "c2", Name: "app-init", State: "exited", ExitCode: &zero, ExpectedCompletion: true},
	}}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		ProjectName: "demo", Attempts: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified {
		t.Fatalf("exited init container with exit code 0 must verify, issues=%v", result.Issues)
	}

	observer.items[0] = append(observer.items[0], ContainerObservation{
		ID: "c3", Name: "boom", State: "exited", ExitCode: &failing,
	})
	result, err = VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		ProjectName: "demo", Attempts: 1,
	})
	if err == nil {
		t.Fatalf("exited container with non-zero exit code must fail verification: %+v", result)
	}
	if result.Verified {
		t.Fatalf("failed verification must not be verified: %+v", result)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Code == "container_not_running" && issue.Container == "boom" && issue.Details["exitCode"] == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("non-zero exit failure must carry container and exit code, issues=%v", result.Issues)
	}
}

func TestVerifyDeploymentRejectsUnexpectedExitZeroService(t *testing.T) {
	zero := 0
	observer := &verificationObserverStub{items: [][]ContainerObservation{{
		{ID: "c1", Name: "proxy", State: "running", Health: "healthy"},
		{ID: "c2", Name: "web", Service: "web", State: "exited", ExitCode: &zero},
	}}}

	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		ProjectName: "demo", Attempts: 1,
	})
	if err == nil || result.Verified {
		t.Fatalf("ordinary service exit 0 must fail verification: result=%#v err=%v", result, err)
	}
	if len(result.Issues) != 1 || result.Issues[0].Code != "container_not_running" || result.Issues[0].Container != "web" {
		t.Fatalf("unexpected issues: %#v", result.Issues)
	}
	if result.Issues[0].Details["service"] != "web" || result.Issues[0].Details["reasonCode"] != "unexpected_service_completion" {
		t.Fatalf("missing actionable completion evidence: %#v", result.Issues)
	}
}

func TestVerifyDeploymentAllowsHealthcheckWarmupRecovery(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{
		{{ID: "web", Name: "web", State: "running", Health: "unhealthy", StartedAtUnix: time.Now().Unix(),
			Healthcheck: &ContainerHealthcheck{Interval: time.Second, Retries: 3}}},
		{{ID: "web", Name: "web", State: "running", Health: "healthy"}},
	}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{ProjectName: "demo", Attempts: 2})
	if err != nil || !result.Verified || observer.calls != 2 {
		t.Fatalf("initial health failure must be allowed to converge: result=%#v err=%v calls=%d", result, err, observer.calls)
	}
}

func TestVerifyDeploymentRejectsUnhealthyAfterWarmup(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{{{
		ID: "web", Name: "web", State: "running", Health: "unhealthy", StartedAtUnix: time.Now().Add(-10 * time.Minute).Unix(),
		Healthcheck: &ContainerHealthcheck{Interval: time.Second, Retries: 3},
	}}}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{ProjectName: "demo", Attempts: 2})
	if err == nil || result.Verified || observer.calls != 1 {
		t.Fatalf("persistent unhealthy must fail: result=%#v err=%v", result, err)
	}
}

func TestVerifyDeploymentKeepsPollingTransientStates(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{
		{{ID: "one", Name: "app", State: "created"}},
		{{ID: "one", Name: "app", State: "restarting"}},
		{{ID: "one", Name: "app", State: "running"}},
	}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      3,
	})
	if err != nil || !result.Verified {
		t.Fatalf("transient states should be polled until running: result=%#v err=%v", result, err)
	}
	if result.Attempts != 3 || observer.calls != 3 {
		t.Fatalf("attempts=%d calls=%d, want 3/3", result.Attempts, observer.calls)
	}
}

func TestVerifyDeploymentDoesNotAcceptRestartLoopRunningSamples(t *testing.T) {
	started := time.Now().Unix()
	observer := &verificationObserverStub{items: [][]ContainerObservation{
		{{ID: "one", Name: "app", State: "running", RestartCount: 0, StartedAtUnix: started}},
		{{ID: "one", Name: "app", State: "restarting", RestartCount: 1, StartedAtUnix: started}},
		{{ID: "one", Name: "app", State: "running", RestartCount: 1, StartedAtUnix: started + 1}},
	}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID:   "local",
		ProjectName:     "demo",
		Attempts:        3,
		StabilityWindow: time.Minute,
	})
	if err == nil || result.Verified {
		t.Fatalf("restart loop must not verify from a transient running sample: result=%#v err=%v", result, err)
	}
	if observer.calls != 3 || result.Attempts != 3 {
		t.Fatalf("restart loop observations=%d attempts=%d, want 3/3", observer.calls, result.Attempts)
	}
	if len(result.Issues) == 0 || result.Issues[0].Code != "container_stabilizing" {
		t.Fatalf("restart loop must expose a stabilizing issue: %#v", result.Issues)
	}
}

func TestVerifyDeploymentAcceptsLongRunningContainerWithoutHealthcheck(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{{{
		ID: "one", Name: "app", State: "running", RestartCount: 2,
		StartedAtUnix: time.Now().Add(-time.Minute).Unix(),
	}}}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID:   "local",
		ProjectName:     "demo",
		Attempts:        1,
		StabilityWindow: 10 * time.Second,
	})
	if err != nil || !result.Verified {
		t.Fatalf("long-running container should verify immediately: result=%#v err=%v", result, err)
	}
}

func TestVerifyDeploymentHealthyContainerDoesNotWaitForStabilityWindow(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{{{
		ID: "one", Name: "app", State: "running", Health: "healthy",
		StartedAtUnix: time.Now().Unix(),
	}}}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		ProjectName: "demo", Attempts: 1, StabilityWindow: time.Minute,
	})
	if err != nil || !result.Verified {
		t.Fatalf("healthy container should use healthcheck evidence: result=%#v err=%v", result, err)
	}
}

func TestVerifyDeploymentStopsEarlyOnExitedContainer(t *testing.T) {
	observer := &verificationObserverStub{items: [][]ContainerObservation{
		{{ID: "one", Name: "app", State: "exited"}},
	}}
	result, err := VerifyDeployment(context.Background(), observer, nil, VerificationRequest{
		EnvironmentID: "local",
		ProjectName:   "demo",
		Attempts:      5,
	})
	if err == nil || result.Verified {
		t.Fatalf("exited container must fail: result=%#v err=%v", result, err)
	}
	if result.Attempts != 1 || observer.calls != 1 {
		t.Fatalf("attempts=%d calls=%d, want 1/1", result.Attempts, observer.calls)
	}
}
