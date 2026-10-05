package api

import (
	"context"
	"dockerpanel/backend/pkg/config"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

var terminalUserPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+(?::[A-Za-z0-9_.-]+)?$`)

func normalizeTerminalUser(raw string) (string, error) {
	user := strings.TrimSpace(raw)
	if user == "" {
		return "", nil
	}
	if len(user) > 128 || !terminalUserPattern.MatchString(user) {
		return "", fmt.Errorf("执行用户仅支持用户名、UID 或 user:group 格式")
	}
	return user, nil
}

func buildTerminalExecConfig(cmdParam, userParam string) (types.ExecConfig, error) {
	shellCmd := strings.Fields(strings.TrimSpace(cmdParam))
	if len(shellCmd) == 0 {
		shellCmd = []string{"/bin/sh"}
	}
	user, err := normalizeTerminalUser(userParam)
	if err != nil {
		return types.ExecConfig{}, err
	}
	return types.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          shellCmd,
		User:         user,
		Env: []string{
			"LANG=C.UTF-8",
			"LC_ALL=C.UTF-8",
			"TERM=xterm-256color",
		},
	}, nil
}

func writeTerminalControl(ws *websocket.Conn, messageType string, data any) {
	_ = ws.WriteJSON(map[string]any{"type": messageType, "data": data})
}

func parseWsMessage(payload []byte) (wsMessage, bool) {
	var msg wsMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return msg, false
	}
	if strings.TrimSpace(msg.Type) == "" {
		return msg, false
	}
	return msg, true
}

func parseStringPayload(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

func parseResizePayload(raw json.RawMessage) (uint, uint, bool) {
	if len(raw) == 0 {
		return 0, 0, false
	}
	var size struct {
		Rows uint `json:"rows"`
		Cols uint `json:"cols"`
	}
	if err := json.Unmarshal(raw, &size); err == nil {
		return size.Rows, size.Cols, true
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err != nil {
		return 0, 0, false
	}
	if err := json.Unmarshal([]byte(asString), &size); err != nil {
		return 0, 0, false
	}
	return size.Rows, size.Cols, true
}

// 添加一个新的终端处理函数，使用Docker SDK直接执行命令
func containerExec(c *gin.Context) {
	containerId := c.Param("id")
	if forbidIfSelfContainer(c, containerId) {
		return
	}
	command := c.Query("cmd")

	if command == "" {
		command = "/bin/sh" // 默认命令
	}

	logging.Info("container command execution started", "container_id", containerId)

	cli, ok := getDockerClient(c)
	if !ok {
		return
	}
	defer cli.Close()

	// 检查容器是否存在并运行
	containerInfo, err := cli.ContainerInspect(context.Background(), containerId)
	if err != nil {
		respondError(c, http.StatusNotFound, "容器不存在", err)
		return
	}

	if !containerInfo.State.Running {
		respondError(c, http.StatusBadRequest, "容器未运行，无法执行命令", nil)
		return
	}

	// 解析命令
	cmdParts := strings.Fields(command)

	// 容器执行命令的配置
	execConfig := types.ExecConfig{
		Cmd:          cmdParts,
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  false, // 不需要输入
		Tty:          false, // 不使用TTY
	}

	// 创建容器执行命令
	execResp, err := cli.ContainerExecCreate(context.Background(), containerId, execConfig)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建exec命令失败", err)
		return
	}

	// 执行容器命令并获取输出
	resp, err := cli.ContainerExecAttach(context.Background(), execResp.ID, types.ExecStartCheck{})
	if err != nil {
		respondError(c, http.StatusInternalServerError, "附加到exec命令失败", err)
		return
	}
	defer resp.Close()

	// 读取所有输出
	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "读取命令输出失败", err)
		return
	}

	// 返回命令输出
	c.JSON(http.StatusOK, gin.H{
		"output":       string(output),
		"command":      command,
		"container_id": containerId,
	})
}

// 定义WebSocket升级器
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")

		// 开发环境允许所有
		if config.IsProduction() {
			// 生产环境检查允许的源
			allowed := config.GetWSOrigins()
			if config.IsAllowedOrigin(origin, allowed) {
				return true
			}

			// 记录拒绝日志
			logging.Warn("terminal WebSocket origin rejected")
			return false
		}

		// 开发环境允许所有
		return true
	},
}

// 添加WebSocket终端处理函数
// containerTerminal 建立与容器的交互式终端会话（WebSocket <-> Docker Exec TTY）
// 行为：
// 1) 从 WebSocket URL 的 cmd 参数读取 shell/入口命令；
// 2) 创建 Docker Exec（TTY=true），桥接容器的输入输出到 WebSocket；
// 3) 支持窗口尺寸调整（type=resize）与持续输入（type=input），保持会话交互直至任一端关闭。
func containerTerminal(c *gin.Context) {
	containerId := c.Param("id")
	if forbidIfSelfContainer(c, containerId) {
		return
	}
	execConfig, err := buildTerminalExecConfig(c.Query("cmd"), c.Query("user"))
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	logging.Info("terminal session requested", "container_id", containerId)

	// 升级HTTP连接为WebSocket
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logging.Warn("terminal WebSocket upgrade failed", "error", err)
		respondError(c, http.StatusInternalServerError, "WebSocket升级失败", err)
		return
	}
	defer ws.Close()

	// 不向TTY写入成功提示，仅在服务端记录
	logging.Debug("terminal WebSocket connected", "container_id", containerId)

	cli, err := docker.NewDockerClient()
	if err != nil {
		errMsg := fmt.Sprintf("Docker客户端创建失败: %v\n", err)
		logging.Error("terminal Docker client creation failed", "error", err)
		writeTerminalControl(ws, "error", map[string]string{"message": strings.TrimSpace(errMsg)})
		return
	}
	defer cli.Close()

	// 检查容器是否存在
	containerInfo, err := cli.ContainerInspect(context.Background(), containerId)
	if err != nil {
		errMsg := fmt.Sprintf("容器不存在或无法访问: %v\n", err)
		logging.Warn("terminal container inspection failed", "container_id", containerId, "error", err)
		writeTerminalControl(ws, "error", map[string]string{"message": strings.TrimSpace(errMsg)})
		return
	}
	if containerInfo.State == nil || !containerInfo.State.Running {
		writeTerminalControl(ws, "error", map[string]string{"message": "容器未运行，无法打开终端"})
		return
	}
	logging.Debug("terminal execution configuration prepared", "container_id", containerId, "custom_user", execConfig.User != "")
	logging.Debug("terminal exec instance creation started", "container_id", containerId)
	// 创建exec实例
	execResp, err := cli.ContainerExecCreate(context.Background(), containerId, execConfig)
	if err != nil {
		errMsg := fmt.Sprintf("创建exec实例失败: %v\r\n", err)
		logging.Error("terminal exec instance creation failed", "container_id", containerId, "error", err)
		writeTerminalControl(ws, "error", map[string]string{"message": strings.TrimSpace(errMsg)})
		return
	}

	logging.Debug("terminal attaching to exec instance", "container_id", containerId)
	// 附加到exec实例
	hijacked, err := cli.ContainerExecAttach(context.Background(), execResp.ID, types.ExecStartCheck{
		Detach: false,
		Tty:    true,
	})
	if err != nil {
		errMsg := fmt.Sprintf("附加到exec实例失败: %v\r\n", err)
		logging.Error("terminal exec attach failed", "container_id", containerId, "error", err)
		writeTerminalControl(ws, "error", map[string]string{"message": strings.TrimSpace(errMsg)})
		return
	}
	defer hijacked.Close()

	logging.Info("terminal session started", "container_id", containerId)
	writeTerminalControl(ws, "ready", map[string]string{
		"shell": strings.Join(execConfig.Cmd, " "),
		"user":  firstNonEmpty(execConfig.User, "容器默认用户"),
	})

	// 处理WebSocket消息
	// 使用互斥锁确保WebSocket写入的线程安全
	var wsWriteMu sync.Mutex

	// 创建一个完成通道，用于同步goroutine（通过 sync.Once 保证只关闭一次）
	done := make(chan struct{})
	var closeOnce sync.Once
	closeDone := func() { closeOnce.Do(func() { close(done) }) }

	// 从容器输出读取并发送到WebSocket
	go func() {
		defer func() {
			logging.Debug("terminal output worker stopped", "container_id", containerId)
			closeDone()
		}()

		buf := make([]byte, 4096)
		for {
			nr, err := hijacked.Reader.Read(buf)
			if err != nil {
				if err != io.EOF {
					wsWriteMu.Lock()
					logging.Debug("terminal output stream closed", "container_id", containerId, "error", err)
					ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("读取容器输出错误: %v\r\n", err)))
					wsWriteMu.Unlock()
				}
				break
			}

			if nr > 0 {
				wsWriteMu.Lock()
				err = ws.WriteMessage(websocket.BinaryMessage, buf[:nr])
				wsWriteMu.Unlock()
				if err != nil {
					logging.Debug("terminal WebSocket output closed", "container_id", containerId, "error", err)
					break
				}
			}
		}
	}()

	// 从WebSocket读取并写入容器输入
	go func() {
		defer func() {
			logging.Debug("terminal input worker stopped", "container_id", containerId)
			// 通知另一个goroutine结束（只关闭一次，避免 panic: close of closed channel）
			closeDone()
		}()

		for {
			messageType, p, err := ws.ReadMessage()
			if err != nil {
				logging.Debug("terminal WebSocket input closed", "container_id", containerId, "error", err)
				break
			}

			if messageType == websocket.TextMessage {
				msg, ok := parseWsMessage(p)
				if !ok {
					logging.Warn("terminal WebSocket message rejected", "container_id", containerId)
					continue
				}
				logging.Debug("terminal WebSocket message received", "container_id", containerId, "type", msg.Type, "size", len(msg.Data))

				switch msg.Type {
				case "input":
					input, ok := parseStringPayload(msg.Data)
					if !ok {
						logging.Warn("terminal input payload rejected", "container_id", containerId)
						continue
					}
					_, err = hijacked.Conn.Write([]byte(input))
					if err != nil {
						wsWriteMu.Lock()
						logging.Debug("terminal container input closed", "container_id", containerId, "error", err)
						ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("写入容器输入错误: %v\r\n", err)))
						wsWriteMu.Unlock()
						break
					}
				case "resize":
					rows, cols, ok := parseResizePayload(msg.Data)
					if !ok {
						logging.Warn("terminal resize payload rejected", "container_id", containerId)
						continue
					}
					logging.Debug("terminal resize requested", "container_id", containerId, "rows", rows, "cols", cols)
					execInfo, inspectErr := cli.ContainerExecInspect(context.Background(), execResp.ID)
					if inspectErr != nil {
						logging.Debug("terminal resize skipped because exec state is unavailable", "container_id", containerId, "error", inspectErr)
						continue
					}
					if !execInfo.Running {
						logging.Debug("terminal resize skipped because exec is not running", "container_id", containerId)
						continue
					}
					err = cli.ContainerExecResize(context.Background(), execResp.ID, types.ResizeOptions{
						Height: rows,
						Width:  cols,
					})
					if err != nil {
						logging.Debug("terminal resize failed", "container_id", containerId, "error", err)
					}
				case "command":
					continue
				}
			} else {
				logging.Debug("terminal non-text WebSocket message ignored", "container_id", containerId, "type", messageType, "size", len(p))
			}
		}
	}()

	// 等待任一goroutine完成
	<-done
	logging.Info("terminal session ended", "container_id", containerId)
}

// 修改路由注册函数，添加替代方法的路由
func RegisterTerminalRoutes(r *gin.RouterGroup) {
	// 仅注册非交互式命令执行路由，终端路由由 RegisterContainerRoutes 提供
	r.GET("/containers/:id/exec", containerExec)
}
