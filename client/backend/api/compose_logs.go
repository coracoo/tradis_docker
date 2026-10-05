package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"dockerpanel/backend/pkg/docker"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
)

type composeLogSSEEvent struct {
	event    string
	data     string
	terminal bool
}

type composeLogDockerClient interface {
	ContainerList(context.Context, types.ContainerListOptions) ([]types.Container, error)
	ContainerInspect(context.Context, string) (types.ContainerJSON, error)
	ContainerLogs(context.Context, string, types.ContainerLogsOptions) (io.ReadCloser, error)
}

func composeLogServiceName(container types.Container) string {
	if service := strings.TrimSpace(container.Labels["com.docker.compose.service"]); service != "" {
		return service
	}
	if len(container.Names) > 0 {
		if name := strings.TrimSpace(strings.TrimPrefix(container.Names[0], "/")); name != "" {
			return name
		}
	}
	if len(container.ID) > 12 {
		return container.ID[:12]
	}
	return container.ID
}

func composeLogShouldFollow(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running", "restarting", "paused":
		return true
	default:
		return false
	}
}

func streamComposeContainerLogs(
	ctx context.Context,
	cli composeLogDockerClient,
	containerSummary types.Container,
	tail string,
	emit func(string),
) error {
	inspect, err := cli.ContainerInspect(ctx, containerSummary.ID)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	logs, err := cli.ContainerLogs(ctx, containerSummary.ID, buildContainerLogOptions(tail, composeLogShouldFollow(containerSummary.State)))
	if err != nil {
		return fmt.Errorf("open container logs: %w", err)
	}
	defer logs.Close()

	var reader io.Reader = logs
	var pipeReader *io.PipeReader
	if inspect.Config == nil || !inspect.Config.Tty {
		var pipeWriter *io.PipeWriter
		pipeReader, pipeWriter = io.Pipe()
		reader = pipeReader
		go func() {
			_, copyErr := stdcopy.StdCopy(pipeWriter, pipeWriter, logs)
			_ = pipeWriter.CloseWithError(copyErr)
		}()
		defer pipeReader.Close()
	}

	service := composeLogServiceName(containerSummary)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := sanitizeRuntimeLogLine(strings.TrimRight(scanner.Text(), "\r\n"))
		if strings.TrimSpace(line) == "" {
			continue
		}
		emit(fmt.Sprintf("%s | %s", service, line))
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("read container logs: %w", err)
	}
	return nil
}

func streamComposeProjectLogs(
	ctx context.Context,
	cli composeLogDockerClient,
	projectName string,
	tail string,
	emit func(string),
) (int, error) {
	containers, err := cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", "com.docker.compose.project="+projectName),
		),
	})
	if err != nil {
		return 0, fmt.Errorf("获取 Compose 项目容器失败: %w", err)
	}

	tail = strings.TrimSpace(tail)
	if tail == "" {
		tail = "200"
	}

	var emitMu sync.Mutex
	safeEmit := func(line string) {
		emitMu.Lock()
		defer emitMu.Unlock()
		emit(line)
	}

	var waitGroup sync.WaitGroup
	for _, item := range containers {
		containerSummary := item
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if err := streamComposeContainerLogs(ctx, cli, containerSummary, tail, safeEmit); err != nil && ctx.Err() == nil {
				safeEmit(fmt.Sprintf("%s | ERROR: %s", composeLogServiceName(containerSummary), err.Error()))
			}
		}()
	}
	waitGroup.Wait()
	return len(containers), nil
}

func streamComposeLogsSSE(c *gin.Context, projectName, tail string) {
	setSSEHeaders(c)
	ctx := c.Request.Context()
	nextID := sseNextIDFromLastEventID(c)

	// 日志是长连接（follow=true 不会自行结束），用专用流式客户端，不占用连接池额度
	cli, err := docker.NewStreamingClient()
	if err != nil {
		sseWriteStringEvent(c, nextID, "stream-error", "连接 Docker 失败: "+err.Error())
		return
	}
	defer cli.Close()

	events := make(chan composeLogSSEEvent, 256)
	go func() {
		count, streamErr := streamComposeProjectLogs(ctx, cli, projectName, tail, func(line string) {
			select {
			case <-ctx.Done():
			case events <- composeLogSSEEvent{event: "message", data: line}:
			}
		})

		terminal := composeLogSSEEvent{event: "complete", data: "日志流已结束", terminal: true}
		if streamErr != nil {
			terminal = composeLogSSEEvent{event: "stream-error", data: streamErr.Error(), terminal: true}
		} else if count == 0 {
			terminal.data = "该 Compose 项目暂无容器日志"
		}
		select {
		case <-ctx.Done():
		case events <- terminal:
		}
		close(events)
	}()

	sseWriteStringEvent(c, nextID, "ready", projectName)
	nextID++
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			_, _ = fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
		case event, ok := <-events:
			if !ok {
				return
			}
			sseWriteStringEvent(c, nextID, event.event, event.data)
			nextID++
			if event.terminal {
				return
			}
		}
	}
}
