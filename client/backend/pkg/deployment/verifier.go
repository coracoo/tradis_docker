package deployment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PublishedPort struct {
	HostIP   string `json:"hostIp,omitempty"`
	HostPort int    `json:"hostPort"`
	Protocol string `json:"protocol"`
}

type ContainerHealthcheck struct {
	Interval    time.Duration `json:"interval"`
	Timeout     time.Duration `json:"timeout"`
	StartPeriod time.Duration `json:"startPeriod"`
	Retries     int           `json:"retries"`
}

type ContainerObservation struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Service     string                `json:"service,omitempty"`
	State       string                `json:"state"`
	Health      string                `json:"health,omitempty"`
	Healthcheck *ContainerHealthcheck `json:"healthcheck,omitempty"`
	// Completion must be expected by the deployed Compose dependency graph.
	ExpectedCompletion bool            `json:"expectedCompletion,omitempty"`
	ExitCode           *int            `json:"exitCode,omitempty"`
	CreatedAtUnix      int64           `json:"createdAtUnix,omitempty"`
	RestartCount       int             `json:"restartCount,omitempty"`
	StartedAtUnix      int64           `json:"startedAtUnix,omitempty"`
	PublishedPorts     []PublishedPort `json:"publishedPorts,omitempty"`
}

type DeploymentObserver interface {
	ObserveDeployment(context.Context, string, string) ([]ContainerObservation, error)
}

type PortProbe interface {
	Probe(context.Context, string, int) error
}

type PortProbeFunc func(context.Context, string, int) error

func (probe PortProbeFunc) Probe(ctx context.Context, host string, port int) error {
	return probe(ctx, host, port)
}

type VerificationRequest struct {
	EnvironmentID   string
	ProjectName     string
	Attempts        int
	Interval        time.Duration
	ProbeTCPPorts   bool
	NotBeforeUnix   int64
	StabilityWindow time.Duration
	OnPending       func(VerificationResult)
}

type ContainerVerification struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Health string `json:"health,omitempty"`
}

type PortVerification struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	Reachable bool   `json:"reachable"`
}

type VerificationIssue struct {
	Code      string         `json:"code"`
	Container string         `json:"container,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type VerificationResult struct {
	Verified   bool                    `json:"verified"`
	Mode       string                  `json:"mode,omitempty"`
	Attempts   int                     `json:"attempts"`
	Containers []ContainerVerification `json:"containers,omitempty"`
	Ports      []PortVerification      `json:"ports,omitempty"`
	Issues     []VerificationIssue     `json:"issues,omitempty"`
}

func VerifyDeployment(
	ctx context.Context,
	observer DeploymentObserver,
	portProbe PortProbe,
	request VerificationRequest,
) (VerificationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request.EnvironmentID = strings.TrimSpace(request.EnvironmentID)
	request.ProjectName = strings.TrimSpace(request.ProjectName)
	if request.EnvironmentID == "" {
		request.EnvironmentID = "local"
	}
	if observer == nil || request.ProjectName == "" {
		return VerificationResult{}, NewStructuredError(
			ErrorCodeInvalidInput,
			errors.New("deployment observer and project name are required"),
			false,
			nil,
			map[string]any{"stage": "verify"},
		)
	}
	if request.Attempts <= 0 {
		request.Attempts = 1
	}
	if request.Attempts > 300 {
		request.Attempts = 300
	}
	if request.Interval < 0 {
		request.Interval = 0
	}

	var result VerificationResult
	attemptLimit := request.Attempts
	for attempt := 1; attempt <= attemptLimit; attempt++ {
		if err := ctx.Err(); err != nil {
			return result, verificationCanceledError(err, attempt)
		}
		observations, err := observer.ObserveDeployment(ctx, request.EnvironmentID, request.ProjectName)
		if err != nil {
			return result, NewStructuredError(
				ErrorCodeVerificationFailed,
				err,
				true,
				[]string{"retry_verification", "inspect_docker"},
				map[string]any{"project": request.ProjectName, "attempt": attempt},
			)
		}
		result = evaluateDeploymentObservations(ctx, observations, portProbe, request.ProbeTCPPorts, request.NotBeforeUnix)
		if result.Verified {
			result = requireDeploymentStability(result, observations, request.StabilityWindow, time.Now())
		}
		result.Attempts = attempt
		if extended := verificationAttemptLimitFromObservations(attemptLimit, request.Interval, observations); extended > attemptLimit {
			attemptLimit = extended
		}
		if result.Verified {
			return result, nil
		}
		if verificationHasTerminalIssue(result.Issues) {
			break
		}
		if request.OnPending != nil {
			request.OnPending(result)
		}
		if attempt < attemptLimit && request.Interval > 0 {
			timer := time.NewTimer(request.Interval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return result, verificationCanceledError(ctx.Err(), attempt)
			case <-timer.C:
			}
		}
	}

	cause := errors.New("deployment verification did not pass")
	if len(result.Issues) > 0 {
		cause = fmt.Errorf("deployment verification failed: %s", result.Issues[0].Code)
	}
	return result, NewStructuredError(
		ErrorCodeVerificationFailed,
		cause,
		false,
		[]string{"inspect_logs", "fix_deployment", "retry"},
		map[string]any{"project": request.ProjectName, "issues": result.Issues},
	)
}

func requireDeploymentStability(
	result VerificationResult,
	observations []ContainerObservation,
	window time.Duration,
	now time.Time,
) VerificationResult {
	if window <= 0 {
		window = 10 * time.Second
	}
	for _, observation := range observations {
		state := strings.ToLower(strings.TrimSpace(observation.State))
		health := strings.ToLower(strings.TrimSpace(observation.Health))
		if health == "none" {
			health = ""
		}
		if state != "running" || health != "" || observation.StartedAtUnix <= 0 {
			continue
		}
		startedAt := time.Unix(observation.StartedAtUnix, 0)
		if !now.Before(startedAt.Add(window)) {
			continue
		}
		result.Verified = false
		result.Issues = append(result.Issues, VerificationIssue{
			Code:      "container_stabilizing",
			Container: strings.TrimPrefix(strings.TrimSpace(observation.Name), "/"),
			Details: map[string]any{
				"restartCount":    observation.RestartCount,
				"startedAtUnix":   observation.StartedAtUnix,
				"requiredSeconds": int64(window / time.Second),
			},
		})
	}
	return result
}

func verificationAttemptLimitFromObservations(current int, interval time.Duration, observations []ContainerObservation) int {
	if current < 1 || interval <= 0 {
		return current
	}
	var longest time.Duration
	for _, observation := range observations {
		window := containerHealthcheckWindow(observation.Healthcheck)
		if window > longest {
			longest = window
		}
	}
	if longest <= 0 {
		return current
	}
	required := int((longest+interval-1)/interval) + 1
	if required > 300 {
		required = 300
	}
	if required > current {
		return required
	}
	return current
}

func containerHealthcheckWindow(config *ContainerHealthcheck) time.Duration {
	if config == nil {
		return 0
	}
	interval, timeout, retries := config.Interval, config.Timeout, config.Retries
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if retries <= 0 {
		retries = 3
	}
	if retries > 100 {
		retries = 100
	}
	window := config.StartPeriod + time.Duration(retries)*interval + timeout
	if window < 2*time.Minute {
		window = 2 * time.Minute
	}
	if window > 5*time.Minute {
		window = 5 * time.Minute
	}
	return window
}

func verificationHasTerminalIssue(issues []VerificationIssue) bool {
	for _, issue := range issues {
		switch issue.Code {
		case "container_unhealthy",
			"container_predates_job",
			"container_creation_unknown",
			"container_health_unknown":
			return true
		case "container_not_running":
			// 仅 exited 视为终态；created/restarting/paused 等瞬时态可能自行恢复，
			// 必须继续轮询到尝试上限，否则初始化期容器会在第一轮就被判失败。
			if state, ok := issue.Details["state"].(string); ok {
				return state == "exited"
			}
		}
	}
	return false
}

func evaluateDeploymentObservations(
	ctx context.Context,
	observations []ContainerObservation,
	portProbe PortProbe,
	probeTCP bool,
	notBeforeUnix int64,
) VerificationResult {
	result := VerificationResult{Verified: true}
	if len(observations) == 0 {
		result.Verified = false
		result.Issues = []VerificationIssue{{Code: "containers_not_found"}}
		return result
	}

	seenPorts := map[string]struct{}{}
	for _, observation := range observations {
		name := strings.TrimPrefix(strings.TrimSpace(observation.Name), "/")
		state := strings.ToLower(strings.TrimSpace(observation.State))
		health := strings.ToLower(strings.TrimSpace(observation.Health))
		if health == "none" {
			health = ""
		}
		result.Containers = append(result.Containers, ContainerVerification{
			ID:     strings.TrimSpace(observation.ID),
			Name:   name,
			State:  state,
			Health: health,
		})
		if notBeforeUnix > 0 {
			if observation.CreatedAtUnix <= 0 {
				result.Verified = false
				result.Issues = append(result.Issues, VerificationIssue{
					Code:      "container_creation_unknown",
					Container: name,
				})
				continue
			}
			if observation.CreatedAtUnix < notBeforeUnix {
				result.Verified = false
				result.Issues = append(result.Issues, VerificationIssue{
					Code:      "container_predates_job",
					Container: name,
					Details: map[string]any{
						"createdAtUnix": observation.CreatedAtUnix,
						"notBeforeUnix": notBeforeUnix,
					},
				})
				continue
			}
		}
		if state != "running" {
			if state == "exited" && observation.ExpectedCompletion && observation.ExitCode != nil && *observation.ExitCode == 0 {
				// depends_on: service_completed_successfully 的 init 容器
				// 跑完即退出，退出码 0 是成功语义，不得判失败。
				continue
			}
			result.Verified = false
			details := map[string]any{"state": state}
			if service := strings.TrimSpace(observation.Service); service != "" {
				details["service"] = service
			}
			if observation.ExitCode != nil {
				details["exitCode"] = *observation.ExitCode
				if state == "exited" && *observation.ExitCode == 0 && !observation.ExpectedCompletion {
					details["reasonCode"] = "unexpected_service_completion"
					details["nextActions"] = []string{"inspect_service_command", "inspect_compose_dependencies"}
					details["hint"] = "Exit 0 is not readiness. Only a verified one-shot initializer with an explicit service_completed_successfully dependency is expected to exit; otherwise repair the long-running service."
				}
			}
			result.Issues = append(result.Issues, VerificationIssue{
				Code:      "container_not_running",
				Container: name,
				Details:   details,
			})
			continue
		}
		switch health {
		case "unhealthy":
			result.Verified = false
			code := "container_unhealthy"
			if window := containerHealthcheckWindow(observation.Healthcheck); window > 0 && observation.StartedAtUnix > 0 && time.Now().Before(time.Unix(observation.StartedAtUnix, 0).Add(window)) {
				code = "container_health_starting"
			}
			result.Issues = append(result.Issues, VerificationIssue{Code: code, Container: name, Details: map[string]any{"health": health}})
		case "starting":
			result.Verified = false
			result.Issues = append(result.Issues, VerificationIssue{Code: "container_health_starting", Container: name})
		case "", "healthy":
		default:
			result.Verified = false
			result.Issues = append(result.Issues, VerificationIssue{
				Code:      "container_health_unknown",
				Container: name,
				Details:   map[string]any{"health": health},
			})
		}
		if !probeTCP || portProbe == nil {
			continue
		}
		for _, published := range observation.PublishedPorts {
			protocol := strings.ToLower(strings.TrimSpace(published.Protocol))
			if protocol == "" {
				protocol = "tcp"
			}
			if protocol != "tcp" || published.HostPort < 1 || published.HostPort > 65535 {
				continue
			}
			host := normalizeProbeHost(published.HostIP)
			key := fmt.Sprintf("%s:%d", host, published.HostPort)
			if _, exists := seenPorts[key]; exists {
				continue
			}
			seenPorts[key] = struct{}{}
			probeResult := PortVerification{Host: host, Port: published.HostPort, Protocol: protocol, Reachable: true}
			if err := portProbe.Probe(ctx, host, published.HostPort); err != nil {
				probeResult.Reachable = false
				result.Verified = false
				result.Issues = append(result.Issues, VerificationIssue{
					Code:      "port_unreachable",
					Container: name,
					Details:   map[string]any{"host": host, "port": published.HostPort, "protocol": protocol},
				})
			}
			result.Ports = append(result.Ports, probeResult)
		}
	}
	return result
}

func normalizeProbeHost(host string) string {
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.Trim(host, "[]")
	}
}

func verificationCanceledError(err error, attempt int) error {
	return NewStructuredError(
		ErrorCodeCanceled,
		err,
		false,
		nil,
		map[string]any{"stage": "verify", "attempt": attempt},
	)
}
