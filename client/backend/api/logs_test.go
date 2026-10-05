package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
)

func TestContainerLogOptionsAlwaysRequestDockerTimestamps(t *testing.T) {
	options := buildContainerLogOptions("200", true)

	if !options.Timestamps {
		t.Fatal("container logs must request Docker timestamps")
	}
	if !options.Follow || options.Tail != "200" {
		t.Fatalf("unexpected log options: %#v", options)
	}
}

func TestComposeLogArgsRequestTimestamps(t *testing.T) {
	args := buildComposeLogArgs("200")
	want := []string{"compose", "logs", "-f", "--timestamps", "--tail", "200"}

	if len(args) != len(want) {
		t.Fatalf("unexpected args: %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("unexpected args: %#v", args)
		}
	}
}

func TestSanitizeRuntimeLogLineUsesCommonRedactor(t *testing.T) {
	line := sanitizeRuntimeLogLine("2026-07-22T13:00:00Z API_TOKEN=runtime-secret service started")
	if strings.Contains(line, "runtime-secret") || !strings.Contains(line, "[REDACTED]") {
		t.Fatalf("runtime log leaked a secret: %q", line)
	}
}

func TestComposeLogsDoNotRequireManagedProjectDirectory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/compose/:name/logs", getComposeLogs)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/compose/external-compose/logs?tail=20", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("discovered Compose logs must not require a local project directory: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("expected SSE response, got %q", contentType)
	}
}

func TestStreamComposeProjectLogsAggregatesContainersByProjectLabel(t *testing.T) {
	var requestedTail string
	var requestedFollow bool
	mock := &docker.MockDockerClient{
		ContainerListFunc: func(_ context.Context, options types.ContainerListOptions) ([]types.Container, error) {
			labels := options.Filters.Get("label")
			if len(labels) != 1 || labels[0] != "com.docker.compose.project=external-compose" {
				t.Fatalf("unexpected project filter: %#v", labels)
			}
			return []types.Container{{
				ID:    "container-1",
				Names: []string{"/external-web-1"},
				State: "running",
				Labels: map[string]string{
					"com.docker.compose.service": "web",
				},
			}}, nil
		},
		ContainerInspectFunc: func(_ context.Context, _ string) (types.ContainerJSON, error) {
			return types.ContainerJSON{Config: &container.Config{Tty: true}}, nil
		},
		ContainerLogsFunc: func(_ context.Context, _ string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
			requestedTail = options.Tail
			requestedFollow = options.Follow
			return io.NopCloser(strings.NewReader("2026-07-18T12:00:00Z server started\n")), nil
		},
	}

	var lines []string
	count, err := streamComposeProjectLogs(context.Background(), mock, "external-compose", "20", func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("stream Compose logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one project container, got %d", count)
	}
	if requestedTail != "20" || !requestedFollow {
		t.Fatalf("unexpected log options: tail=%q follow=%v", requestedTail, requestedFollow)
	}
	if len(lines) != 1 || lines[0] != "web | 2026-07-18T12:00:00Z server started" {
		t.Fatalf("unexpected aggregated lines: %#v", lines)
	}
}

func TestStreamComposeProjectLogsDemultiplexesStoppedContainerHistory(t *testing.T) {
	var payload bytes.Buffer
	stdout := stdcopy.NewStdWriter(&payload, stdcopy.Stdout)
	_, _ = stdout.Write([]byte("2026-07-18T12:00:00Z stopped cleanly\n"))

	mock := &docker.MockDockerClient{
		ContainerListFunc: func(_ context.Context, _ types.ContainerListOptions) ([]types.Container, error) {
			return []types.Container{{ID: "container-2", Names: []string{"/worker-1"}, State: "exited"}}, nil
		},
		ContainerInspectFunc: func(_ context.Context, _ string) (types.ContainerJSON, error) {
			return types.ContainerJSON{Config: &container.Config{Tty: false}}, nil
		},
		ContainerLogsFunc: func(_ context.Context, _ string, options types.ContainerLogsOptions) (io.ReadCloser, error) {
			if options.Follow {
				t.Fatal("stopped container history must not use follow mode")
			}
			return io.NopCloser(bytes.NewReader(payload.Bytes())), nil
		},
	}

	var lines []string
	_, err := streamComposeProjectLogs(context.Background(), mock, "external-compose", "20", func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatalf("stream stopped Compose logs: %v", err)
	}
	if len(lines) != 1 || lines[0] != "worker-1 | 2026-07-18T12:00:00Z stopped cleanly" {
		t.Fatalf("unexpected stopped container logs: %#v", lines)
	}
}
