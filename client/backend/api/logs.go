package api

import (
	"bufio"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/secrets"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
)

// 每次写入后刷新响应的 Writer，确保日志实时推送到前端
type flushWriter struct{ w gin.ResponseWriter }

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.w.Flush()
	return n, err
}

func buildContainerLogOptions(tail string, follow bool) types.ContainerLogsOptions {
	return types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: true,
		Tail:       tail,
	}
}

func sanitizeRuntimeLogLine(line string) string {
	return secrets.RedactString(line)
}

// 修改日志接口，支持实时日志
func getContainerLogs(c *gin.Context) {
	// 使用请求上下文，客户端断开连接时取消 Follow 流，避免 goroutine 与 Docker 流泄漏
	ctx := c.Request.Context()

	id := c.Param("id")
	if forbidIfSelfContainer(c, id) {
		return
	}

	// 日志是长连接（follow=true 不会自行结束），用专用流式客户端，不占用连接池额度
	cli, err := docker.NewStreamingClient()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "连接Docker失败", err)
		return
	}
	defer cli.Close()

	// 先检查容器是否存在，并获取 TTY 配置
	inspect, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		respondError(c, http.StatusNotFound, "容器不存在", err)
		return
	}

	options := buildContainerLogOptions("100", true)

	logs, err := cli.ContainerLogs(ctx, id, options)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取容器日志失败", err)
		return
	}
	defer logs.Close()

	// 使用纯文本流，避免事件流格式导致前端解析异常
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	fw := flushWriter{w: c.Writer}

	reader := io.Reader(logs)
	if inspect.Config == nil || !inspect.Config.Tty {
		pipeReader, pipeWriter := io.Pipe()
		reader = pipeReader
		go func() {
			_, copyErr := stdcopy.StdCopy(pipeWriter, pipeWriter, logs)
			_ = pipeWriter.CloseWithError(copyErr)
		}()
		defer pipeReader.Close()
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		_, _ = fmt.Fprintln(fw, sanitizeRuntimeLogLine(strings.TrimRight(scanner.Text(), "\r\n")))
	}
}

// getContainerLogsEvents 以 SSE 方式推送容器日志（逐行）
func getContainerLogsEvents(c *gin.Context) {
	ctx := c.Request.Context()

	id := c.Param("id")

	// 日志查看是只读操作，不做自我容器保护（修改类操作如启停/终端仍受 forbidIfSelfContainer 约束）。
	// 与 Compose 日志路由行为一致：允许查看面板自身容器的日志。
	setSSEHeaders(c)
	nextID := sseNextIDFromLastEventID(c)

	// SSE 日志流长期占用连接，用专用流式客户端，不占用连接池额度
	cli, err := docker.NewStreamingClient()
	if err != nil {
		sseWriteStringEvent(c, nextID, "stream-error", "连接 Docker 失败: "+err.Error())
		return
	}
	defer cli.Close()

	inspect, err := cli.ContainerInspect(ctx, id)
	if err != nil {
		sseWriteStringEvent(c, nextID, "stream-error", "容器不存在或无法访问: "+err.Error())
		return
	}

	state := ""
	if inspect.State != nil {
		state = inspect.State.Status
	}

	tail := strings.TrimSpace(c.DefaultQuery("tail", "200"))
	if tail == "" {
		tail = "200"
	}

	// 已停止容器 follow=false，读完历史日志后关闭流并发送 complete 终止事件，
	// 避免 follow=true 长挂导致前端 onerror 触发自动重连死循环
	options := buildContainerLogOptions(tail, composeLogShouldFollow(state))

	logs, err := cli.ContainerLogs(ctx, id, options)
	if err != nil {
		sseWriteStringEvent(c, nextID, "stream-error", "获取容器日志失败: "+err.Error())
		return
	}
	defer logs.Close()

	events := make(chan composeLogSSEEvent, 256)
	go func() {
		defer close(events)

		lines := make(chan string, 256)
		pr, pw := io.Pipe()

		go func() {
			defer func() { _ = pw.Close() }()

			if inspect.Config != nil && inspect.Config.Tty {
				_, _ = io.Copy(pw, logs)
				return
			}
			_, _ = stdcopy.StdCopy(pw, pw, logs)
		}()

		go func() {
			defer close(lines)
			defer func() { _ = pr.Close() }()

			scanner := bufio.NewScanner(pr)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				select {
				case <-ctx.Done():
					return
				default:
				}
				line := sanitizeRuntimeLogLine(strings.TrimRight(scanner.Text(), "\r\n"))
				if strings.TrimSpace(line) == "" {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case lines <- line:
				}
			}
		}()

		for line := range lines {
			select {
			case <-ctx.Done():
				return
			case events <- composeLogSSEEvent{event: "message", data: line}:
			}
		}

		terminal := composeLogSSEEvent{event: "complete", data: "日志流已结束", terminal: true}
		select {
		case <-ctx.Done():
		case events <- terminal:
		}
	}()

	sseWriteStringEvent(c, nextID, "ready", id)
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
