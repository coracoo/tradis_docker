// cmd/main.go
package main

import (
	"context"
	"dockerpanel/backend/api"
	"dockerpanel/backend/pkg/admincli"
	"dockerpanel/backend/pkg/background"
	"dockerpanel/backend/pkg/cloudflare"
	"dockerpanel/backend/pkg/config"
	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/docker"
	"dockerpanel/backend/pkg/logging"
	"dockerpanel/backend/pkg/scheduler"
	"dockerpanel/backend/pkg/secrets"
	"dockerpanel/backend/pkg/settings"
	"dockerpanel/backend/pkg/system"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const backgroundShutdownTimeout = 30 * time.Second

// trustedProxiesFromEnv keeps forwarded headers untrusted unless an operator
// explicitly configures the reverse proxy address or CIDR.
func trustedProxiesFromEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"127.0.0.1", "::1"}
	}
	proxies := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item != "" {
			proxies = append(proxies, item)
		}
	}
	return proxies
}

func registerClientAPIRoutes(r *gin.Engine) {
	api.RegisterAuthRoutes(r)

	// 公开健康检查：updater 容器用它核验新版本面板是否就绪。
	r.GET("/api/health", api.HealthHandler)

	// WebSocket 也必须经过同一套认证与 edition 路由边界。完整版本
	// 增加远程环境中间件；community 构建只保留本地请求。
	protected := r.Group("/api")
	protected.Use(api.AuthMiddleware())
	registerEditionRequestMiddleware(protected)

	api.RegisterCommunityRoutes(r, protected)
	registerEditionRoutes(protected)
}

func main() {
	// Full build owns the updater mode. Community deliberately has no
	// auto-update entry until it has an independent signed release source.
	if handled, exitCode := handleEditionUpdater(); handled {
		os.Exit(exitCode)
	}

	if !settings.IsDebugEnabled() {
		gin.SetMode(gin.ReleaseMode)
	}

	if handled, err := admincli.Run(os.Args[1:], os.Stdin, os.Stdout); handled {
		if err != nil {
			logging.Error("administrator command failed", "error", err)
			os.Exit(1)
		}
		return
	}

	// 设置数据目录
	dataDir := settings.GetDataDir()
	logging.Info("data directory selected", "path", dataDir)
	if err := secrets.ConfigurePersistentMasterKey(dataDir); err != nil {
		logging.Error("persistent master key initialization failed", "error", err)
		os.Exit(1)
	}

	// 初始化数据库
	logging.Info("database initialization started")
	dbPath := filepath.Join(dataDir, "data.db") // 指定数据库文件路径
	if err := database.InitDB(dbPath); err != nil {
		logging.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	logging.Info("database initialization completed")

	// 初始化Docker客户端连接池
	logging.Info("Docker client pool initialization started")
	if err := docker.InitPool(); err != nil {
		logging.Warn("Docker client pool initialization failed; direct connections will be used", "error", err)
		// 非致命错误，可以继续运行（会回退到直接创建连接）
	} else {
		logging.Info("Docker client pool initialization completed")
	}

	// 初始化全局设置默认值
	if err := settings.InitSettingsTable(); err != nil {
		logging.Error("global settings initialization failed", "error", err)
	}

	// 每次启动都校准宿主机 Docker 配置；数据库仅作为主机配置不可读时的兜底。
	syncContext, cancelDockerConfigSync := context.WithTimeout(context.Background(), 5*time.Second)
	if err := api.SyncHostDockerNetworkConfig(syncContext); err != nil {
		logging.Warn("host Docker network configuration synchronization failed", "error", err)
	} else {
		logging.Info("host Docker network configuration synchronized")
	}
	cancelDockerConfigSync()

	// 启动 Docker 事件日志记录器
	system.StartEventLogger()
	api.InitClientVersionFromEnv()
	if err := prepareEditionRuntime(); err != nil {
		logging.Error("edition runtime preparation failed", "error", err)
		os.Exit(1)
	}
	api.StartImageUpdateScheduler()

	// Both edition callback sets must be registered before scheduler.Start.
	api.RegisterCommunitySchedulerCallbacks()
	registerEditionSchedulerCallbacks()
	scheduler.Start()

	// 启动后立即异步准备 CloudflareSpeedTest 二进制。
	// 未配置 CDN 时不会测速；后续保存 CDN 配置会自动触发后台测速。
	cloudflare.StartBackgroundSpeedTest()

	rootContext, cancelBackground := context.WithCancel(context.Background())
	settingsRunner := background.New(rootContext)
	api.SetSettingsBackgroundRunner(settingsRunner)
	cpuSampler := system.NewCPUSampler(nil, time.Second, 5*time.Second)
	api.SetHostCPUSnapshotProvider(cpuSampler.Current)
	if !settingsRunner.Submit("system.cpu-sampler", cpuSampler.Run) {
		logging.Error("host CPU sampler lifecycle registration failed")
		os.Exit(1)
	}
	if err := startEditionBackground(rootContext, settingsRunner); err != nil {
		logging.Error("edition background startup failed", "error", err)
		os.Exit(1)
	}

	r := gin.New()
	r.Use(logging.GinMiddleware(), logging.GinRecovery())

	trustedProxies := trustedProxiesFromEnv(os.Getenv("TRUSTED_PROXIES"))
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		logging.Error("trusted proxy configuration failed", "error", err)
	} else {
		logging.Debug("trusted proxy configuration applied", "count", len(trustedProxies))
	}

	// Configure CORS
	corsConfig := cors.DefaultConfig()

	if os.Getenv("GIN_MODE") == "release" {
		// 生产环境使用配置的源
		allowedOrigins := config.GetCORSOrigins()
		allowAllOrigins := false
		for _, origin := range allowedOrigins {
			if strings.TrimSpace(origin) == "*" {
				allowAllOrigins = true
				break
			}
		}
		if allowAllOrigins {
			// 浏览器禁止 "*" 与凭据模式组合。公开所有来源时必须关闭跨域 Cookie。
			corsConfig.AllowAllOrigins = true
			corsConfig.AllowCredentials = false
			logging.Warn("CORS allows all origins in production; cross-origin credentials are disabled")
		} else {
			corsConfig.AllowOriginFunc = func(origin string) bool {
				return config.IsAllowedOrigin(origin, allowedOrigins)
			}
			corsConfig.AllowCredentials = true
			logging.Info("production CORS configuration applied", "origin_count", len(allowedOrigins))
		}
		corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}
	} else {
		// 开发环境允许所有
		corsConfig.AllowAllOrigins = true
		logging.Debug("development CORS configuration allows all origins")
	}
	corsConfig.AllowHeaders = append([]string{
		"Origin", "Content-Type", "Accept", "Authorization",
		"X-Requested-With", "X-CSRF-Token",
	}, editionCORSAllowedHeaders()...)
	corsConfig.ExposeHeaders = editionCORSExposedHeaders()

	r.Use(cors.New(corsConfig))

	registerClientAPIRoutes(r)

	recoverEditionState(rootContext)
	api.RecoverCleanupTasks()

	// 启动发现与设置变更共用同一生命周期管理器。
	if !settingsRunner.Submit("settings.container-discovery", func(ctx context.Context) error {
		system.ProcessContainerDiscoveryContext(ctx)
		return nil
	}) {
		logging.Warn("startup container discovery was not scheduled")
	}
	startEditionRecurring()
	// 启动容器事件监听
	go system.WatchContainerEvents()

	// 静态文件服务
	// 1. 静态资源 (assets) - 对应 dist/assets 目录
	r.Static("/assets", "./dist/assets")
	// 1.1 兼容旧路径：上传的导航图标
	r.Static("/uploads/icons", filepath.Join(settings.GetDataDir(), "icons"))
	// 1.2 新路径：统一静态图片目录 /data/pic
	r.Static("/data/pic", filepath.Join(settings.GetDataDir(), "pic"))

	// 2. 根路径路由
	r.GET("/", func(c *gin.Context) {
		c.File("./dist/index.html")
	})

	// 3. SPA 回退逻辑 (NoRoute)
	// 处理所有未匹配的路由，支持 History 模式路由和根目录静态文件
	r.NoRoute(func(c *gin.Context) {
		// 避免 API 请求被错误返回 index.html
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(404, gin.H{"error": "API not found"})
			return
		}

		// 检查文件是否存在于 dist 根目录中 (例如 favicon.ico, vite.svg)
		// 注意：这里需要防止目录遍历攻击，但 filepath.Join 和 Clean 通常能处理
		path := filepath.Join("./dist", c.Request.URL.Path)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			c.File(path)
			return
		}

		// 默认返回 index.html (SPA 支持)
		c.File("./dist/index.html")
	})

	port := strings.TrimSpace(os.Getenv("BACKEND_PORT"))
	logging.Debug("backend port configuration read", "configured", port != "")

	if port == "" {
		port = "8080"
		logging.Debug("default backend port selected")
	}

	bind := os.Getenv("BACKEND_BIND")
	if bind == "" {
		bind = fmt.Sprintf(":%s", port)
	}

	server := &http.Server{Addr: bind, Handler: r, BaseContext: func(net.Listener) context.Context { return rootContext }}
	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	serverErrors := make(chan error, 1)
	logging.Info("HTTP server starting", "port", port)
	go func() { serverErrors <- server.ListenAndServe() }()

	var serveErr error
	select {
	case <-signalContext.Done():
		logging.Info("shutdown signal received")
	case serveErr = <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			logging.Error("HTTP server stopped unexpectedly", "error", serveErr)
		}
	}
	shutdownClient(server, cancelBackground, settingsRunner, backgroundShutdownTimeout, database.Close)
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		os.Exit(1)
	}
}

func shutdownClient(server *http.Server, cancelBackground context.CancelFunc, runner *background.Runner, timeout time.Duration, closeDatabase func() error) {
	shutdownClientWithTimeoutObserver(server, cancelBackground, runner, timeout, closeDatabase, nil)
}

type shutdownTimeoutObserver func(component string)

func shutdownClientWithTimeoutObserver(server *http.Server, cancelBackground context.CancelFunc, runner *background.Runner, timeout time.Duration, closeDatabase func() error, observeTimeout shutdownTimeoutObserver) {
	if timeout <= 0 {
		timeout = backgroundShutdownTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	httpDone := shutdownHTTPServer(ctx, server)
	if cancelBackground != nil {
		cancelBackground()
	}
	runnerDone := shutdownBackgroundRunner(ctx, runner)
	httpErr := <-httpDone
	runnerErr := <-runnerDone
	for component, err := range map[string]error{"http": httpErr, "settings_background": runnerErr} {
		if errors.Is(err, context.DeadlineExceeded) && observeTimeout != nil {
			observeTimeout(component)
		}
	}
	if httpErr != nil && !errors.Is(httpErr, http.ErrServerClosed) {
		logging.Error("HTTP server shutdown failed", "error", httpErr)
	}
	if runnerErr != nil {
		logging.Error("settings background shutdown failed", "error", runnerErr)
	}
	if httpErr != nil || runnerErr != nil {
		if server != nil {
			_ = server.Close()
		}
		// An uncooperative handler/task may still write. Let process exit release
		// the database instead of closing it concurrently with those writers.
		logging.Error("shutdown incomplete; leaving database open until process exit")
		return
	}
	if closeDatabase != nil {
		if err := closeDatabase(); err != nil {
			logging.Error("database shutdown failed", "error", err)
		}
	}
}

func shutdownHTTPServer(ctx context.Context, server *http.Server) <-chan error {
	done := make(chan error, 1)
	if server == nil {
		done <- nil
		return done
	}

	stopped := make(chan struct{})
	var stoppedOnce sync.Once
	server.RegisterOnShutdown(func() {
		stoppedOnce.Do(func() { close(stopped) })
	})
	go func() {
		done <- server.Shutdown(ctx)
	}()
	<-stopped
	return done
}

func shutdownBackgroundRunner(ctx context.Context, runner *background.Runner) <-chan error {
	done := make(chan error, 1)
	if runner == nil {
		done <- nil
		return done
	}
	go func() {
		done <- runner.Shutdown(ctx)
	}()
	return done
}
