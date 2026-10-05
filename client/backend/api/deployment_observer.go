package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/deployment"
	tradisdocker "dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
)

// dockerDeploymentObserver is the local Compose verifier shared by manual
// Compose and full-product deployment workflows. It contains no Agent or
// source-provider behavior.
type dockerDeploymentObserver struct{}

func (dockerDeploymentObserver) ObserveDeployment(ctx context.Context, environmentID string, projectName string) ([]deployment.ContainerObservation, error) {
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		environmentID = database.LocalEnvironmentID
	}
	if environmentID != database.LocalEnvironmentID {
		return nil, fmt.Errorf("部署环境 %s 尚未接入 Docker 观察器", environmentID)
	}
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return nil, errors.New("部署项目名称不能为空")
	}

	observations := make([]deployment.ContainerObservation, 0)
	err := tradisdocker.WithClient(ctx, func(client *tradisdocker.Client) error {
		containers, err := client.ContainerList(ctx, types.ContainerListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("label", "com.docker.compose.project="+projectName),
			),
		})
		if err != nil {
			return err
		}
		managedWorkingDir := ""
		if dir, dirErr := resolveProjectDir(getProjectsBaseDir(), projectName); dirErr == nil {
			managedWorkingDir = canonicalComposeProjectPath(dir, projectName)
		}
		observations = make([]deployment.ContainerObservation, 0, len(containers))
		completedServices := deploymentCompletedServices(containers, projectName, managedWorkingDir)
		for _, container := range containers {
			if !composeObservationMatchesManagedProject(container.Labels, projectName, managedWorkingDir) {
				continue
			}
			inspect, inspectErr := client.ContainerInspect(ctx, container.ID)
			if inspectErr != nil {
				return inspectErr
			}
			observation := buildDeploymentContainerObservation(container, inspect)
			observation.ExpectedCompletion = completedServices[container.Labels["com.docker.compose.service"]]
			observations = append(observations, observation)
		}
		return nil
	})
	return observations, err
}

func deploymentCompletedServices(containers []types.Container, projectName, managedWorkingDir string) map[string]bool {
	completed := map[string]bool{}
	for _, container := range containers {
		if !composeObservationMatchesManagedProject(container.Labels, projectName, managedWorkingDir) {
			continue
		}
		for _, dependency := range strings.Split(container.Labels["com.docker.compose.depends_on"], ",") {
			parts := strings.Split(dependency, ":")
			if len(parts) >= 2 && parts[0] != "" && parts[1] == "service_completed_successfully" {
				completed[parts[0]] = true
			}
		}
	}
	return completed
}

func composeObservationMatchesManagedProject(labels map[string]string, projectName, managedWorkingDir string) bool {
	if managedWorkingDir == "" {
		return true
	}
	workdir := canonicalComposeObservedProjectPath(
		strings.TrimSpace(labels["com.docker.compose.project.working_dir"]), projectName)
	if workdir == "" {
		return true
	}
	return workdir == managedWorkingDir
}

func buildDeploymentContainerObservation(container types.Container, inspect types.ContainerJSON) deployment.ContainerObservation {
	name := ""
	if len(container.Names) > 0 {
		name = strings.TrimPrefix(strings.TrimSpace(container.Names[0]), "/")
	}
	if name == "" {
		name = strings.TrimPrefix(strings.TrimSpace(inspect.Name), "/")
	}
	if name == "" {
		name = strings.TrimSpace(container.ID)
		if len(name) > 12 {
			name = name[:12]
		}
	}
	health := ""
	if inspect.State != nil && inspect.State.Health != nil {
		health = strings.ToLower(strings.TrimSpace(inspect.State.Health.Status))
	}
	var healthcheck *deployment.ContainerHealthcheck
	if inspect.Config != nil && inspect.Config.Healthcheck != nil &&
		(len(inspect.Config.Healthcheck.Test) == 0 || !strings.EqualFold(strings.TrimSpace(inspect.Config.Healthcheck.Test[0]), "NONE")) {
		healthcheck = &deployment.ContainerHealthcheck{
			Interval:    inspect.Config.Healthcheck.Interval,
			Timeout:     inspect.Config.Healthcheck.Timeout,
			StartPeriod: inspect.Config.Healthcheck.StartPeriod,
			Retries:     inspect.Config.Healthcheck.Retries,
		}
	}
	var exitCode *int
	var startedAtUnix int64
	if inspect.State != nil && !inspect.State.Running && strings.EqualFold(strings.TrimSpace(inspect.State.Status), "exited") {
		code := int(inspect.State.ExitCode)
		exitCode = &code
	}
	if inspect.State != nil {
		if startedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(inspect.State.StartedAt)); err == nil && startedAt.Unix() > 0 {
			startedAtUnix = startedAt.Unix()
		}
	}
	ports := make([]deployment.PublishedPort, 0, len(container.Ports))
	for _, port := range container.Ports {
		if port.PublicPort == 0 {
			continue
		}
		ports = append(ports, deployment.PublishedPort{
			HostIP:   strings.TrimSpace(port.IP),
			HostPort: int(port.PublicPort),
			Protocol: strings.ToLower(strings.TrimSpace(port.Type)),
		})
	}
	return deployment.ContainerObservation{
		ID:             strings.TrimSpace(container.ID),
		Name:           name,
		Service:        strings.TrimSpace(container.Labels["com.docker.compose.service"]),
		State:          strings.ToLower(strings.TrimSpace(container.State)),
		Health:         health,
		Healthcheck:    healthcheck,
		ExitCode:       exitCode,
		CreatedAtUnix:  container.Created,
		RestartCount:   inspect.RestartCount,
		StartedAtUnix:  startedAtUnix,
		PublishedPorts: ports,
	}
}

type tcpDeploymentPortProbe struct {
	timeout time.Duration
}

func (probe tcpDeploymentPortProbe) Probe(ctx context.Context, host string, port int) error {
	timeout := probe.timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return err
	}
	return connection.Close()
}
