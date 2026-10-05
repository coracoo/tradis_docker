package database

import (
	"context"
	"database/sql"
	"dockerpanel/backend/pkg/logging"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dockerpanel/backend/pkg/secrets"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

var ErrTaskRequestConflict = errors.New("task idempotency key was already used with a different request")

var notificationEmitter = struct {
	sync.RWMutex
	fn func(Notification)
}{}

type columnSpec struct {
	Name         string
	AddColumnSQL string
	BackfillSQL  []string
}

// InitDB 初始化数据库连接
func InitDB(dbPath string) error {
	// 确保数据目录存在
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	log.Printf("正在打开数据库: %s", dbPath)

	// 通过 DSN 参数配置 SQLite，确保连接池中的每条连接都生效：
	//   _busy_timeout: 写事务遇锁时等待（毫秒），而非立即返回 "database is locked"。
	//                  WAL 模式下并发写的标准配套，避免短暂锁竞争导致操作失败。
	//   _txlock: 写事务使用 IMMEDIATE 锁，减少升级死锁概率。
	dsn := dbPath + "?_busy_timeout=5000&_txlock=immediate"
	var err error
	db, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return err
	}

	// 配置连接池
	configurePool()

	// 启用WAL模式提升并发性能
	if err := enableWAL(); err != nil {
		log.Printf("[警告] 启用WAL模式失败: %v", err)
		// WAL模式失败不是致命错误，继续运行
	}

	// 测试数据库连接
	if err := db.Ping(); err != nil {
		return err
	}

	log.Printf("[数据库] 连接池配置完成，WAL模式已启用")

	// 创建表
	return createTables()
}

// configurePool 配置数据库连接池
func configurePool() {
	// 最大打开连接数
	maxOpen := 25
	if env := os.Getenv("DB_MAX_OPEN_CONNS"); env != "" {
		if n, err := parseInt(env); err == nil && n > 0 {
			maxOpen = n
		}
	}
	db.SetMaxOpenConns(maxOpen)

	// 最大空闲连接数
	maxIdle := 10
	if env := os.Getenv("DB_MAX_IDLE_CONNS"); env != "" {
		if n, err := parseInt(env); err == nil && n >= 0 {
			maxIdle = n
		}
	}
	db.SetMaxIdleConns(maxIdle)

	// 连接最大生命周期
	maxLifetime := 5 * time.Minute
	if env := os.Getenv("DB_CONN_MAX_LIFETIME"); env != "" {
		if d, err := time.ParseDuration(env); err == nil && d > 0 {
			maxLifetime = d
		}
	}
	db.SetConnMaxLifetime(maxLifetime)

	log.Printf("[数据库] 连接池配置: maxOpen=%d, maxIdle=%d, maxLifetime=%v",
		maxOpen, maxIdle, maxLifetime)
}

// enableWAL 启用WAL模式
func enableWAL() error {
	// WAL模式提升并发写入性能
	_, err := db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		return err
	}

	// 同步模式设置为NORMAL，平衡性能和数据安全
	_, err = db.Exec("PRAGMA synchronous=NORMAL")
	if err != nil {
		return err
	}

	// 检查WAL模式是否启用
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		return err
	}

	if journalMode != "wal" {
		return fmt.Errorf("WAL模式未启用，当前模式: %s", journalMode)
	}

	return nil
}

// parseInt 解析整数
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// ensureTableColumns 确保指定表包含必要列，兼容旧数据库升级。
func ensureTableColumns(table string, cols []columnSpec) error {
	// 严格校验表名（只允许字母、数字、下划线）
	if !isValidIdentifier(table) {
		return fmt.Errorf("非法表名: %s", table)
	}

	// 使用参数化方式无法用于PRAGMA，只能拼接
	// 但已校验表名安全
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	exists := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		exists[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, c := range cols {
		if c.Name == "" || strings.TrimSpace(c.AddColumnSQL) == "" {
			continue
		}
		if exists[c.Name] {
			continue
		}

		// 校验列定义格式
		if !isValidColumnDef(c.AddColumnSQL) {
			return fmt.Errorf("非法列定义: %s", c.AddColumnSQL)
		}

		// 安全的ALTER TABLE执行
		sql := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, c.AddColumnSQL)
		if _, err := db.Exec(sql); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
				continue
			}
			return err
		}

		// 执行回填SQL（需单独校验）
		for _, bf := range c.BackfillSQL {
			if strings.TrimSpace(bf) == "" {
				continue
			}
			// 只允许UPDATE语句
			if !isValidBackfillSQL(bf, table) {
				return fmt.Errorf("非法回填SQL: %s", bf)
			}
			if _, err := db.Exec(bf); err != nil {
				return err
			}
		}
	}
	return nil
}

// isValidIdentifier 校验标识符（表名、列名）
// 只允许字母、数字、下划线，且必须以字母或下划线开头
func isValidIdentifier(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// 必须以字母或下划线开头
	first := s[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	// 其余字符只能是字母、数字、下划线
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}

	return true
}

// isValidColumnDef 校验列定义格式
// 只允许: column_name DATA_TYPE [constraints]
// 其中 DATA_TYPE 必须是SQLite支持的基本类型
func isValidColumnDef(def string) bool {
	def = strings.TrimSpace(def)
	if def == "" {
		return false
	}

	// 分割列名和类型定义
	parts := strings.Fields(def)
	if len(parts) < 2 {
		return false
	}

	// 校验列名
	if !isValidIdentifier(parts[0]) {
		return false
	}

	// 校验数据类型（必须是SQLite支持的基本类型）
	validTypes := map[string]bool{
		"INTEGER": true, "INT": true,
		"TEXT": true, "VARCHAR": true, "CHAR": true,
		"REAL": true, "FLOAT": true, "DOUBLE": true,
		"NUMERIC": true, "DECIMAL": true,
		"BLOB":    true,
		"BOOLEAN": true, "BOOL": true,
		"DATETIME": true, "DATE": true, "TIME": true,
	}

	typeName := strings.ToUpper(parts[1])
	// 移除类型后面的长度定义，如 VARCHAR(255)
	if idx := strings.Index(typeName, "("); idx > 0 {
		typeName = typeName[:idx]
	}

	if !validTypes[typeName] {
		return false
	}

	// 检查约束（可选）
	for i := 2; i < len(parts); i++ {
		upper := strings.ToUpper(parts[i])
		switch upper {
		case "PRIMARY", "KEY", "UNIQUE", "NOT", "NULL", "DEFAULT",
			"CHECK", "REFERENCES", "COLLATE", "AUTOINCREMENT":
			// 合法约束关键字
		default:
			// 检查是否是 DEFAULT 值（如 DEFAULT 'value' 或 DEFAULT 0）
			if i >= 3 && strings.ToUpper(parts[i-1]) == "DEFAULT" {
				continue
			}
			// 检查是否是外键引用
			if i >= 2 && strings.ToUpper(parts[i-1]) == "REFERENCES" {
				if isValidIdentifier(parts[i]) {
					continue
				}
			}
			// 未知关键字，拒绝
			return false
		}
	}

	return true
}

// isValidBackfillSQL 校验回填SQL
// 只允许 UPDATE table_name SET ... WHERE ...
func isValidBackfillSQL(sql, table string) bool {
	sql = strings.TrimSpace(sql)
	upper := strings.ToUpper(sql)

	// 必须是UPDATE语句
	if !strings.HasPrefix(upper, "UPDATE ") {
		return false
	}

	// 检查是否只操作指定的表
	expected := fmt.Sprintf("UPDATE %s", table)
	if !strings.HasPrefix(upper, strings.ToUpper(expected)) {
		return false
	}

	// 检查禁止的操作
	forbidden := []string{";", "--", "/*", "*/", "DROP", "DELETE", "INSERT", "SELECT"}
	for _, f := range forbidden {
		if strings.Contains(upper, f) {
			return false
		}
	}

	return true
}

// createTables 创建必要的数据库表
func createTables() error {
	var err error
	// 创建用户表
	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS users (
	        id INTEGER PRIMARY KEY AUTOINCREMENT,
	        username TEXT NOT NULL UNIQUE,
	        password TEXT NOT NULL,
	        token_version INTEGER NOT NULL DEFAULT 1,
	        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	    );
    `)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("users", []columnSpec{
		{Name: "token_version", AddColumnSQL: "token_version INTEGER NOT NULL DEFAULT 1"},
	}); err != nil {
		return err
	}

	// 初始化管理员账户
	if err = initAdminUser(); err != nil {
		log.Printf("警告: 初始化管理员账户失败: %v", err)
	}
	if err := maybeResetAdminPassword(); err != nil {
		log.Printf("警告: 重置管理员密码失败: %v", err)
	}

	// 创建注册表配置表
	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS registries (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        url TEXT NOT NULL,
        username TEXT,
        password TEXT,
        is_default INTEGER DEFAULT 0,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("registries", []columnSpec{
		{Name: "username", AddColumnSQL: "username TEXT"},
		{Name: "password", AddColumnSQL: "password TEXT"},
		{Name: "is_default", AddColumnSQL: "is_default INTEGER DEFAULT 0"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME"},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME"},
	}); err != nil {
		return err
	}

	// 创建 Docker 代理配置表
	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS docker_proxy (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        enabled INTEGER DEFAULT 0,
        http_proxy TEXT,
        https_proxy TEXT,
        no_proxy TEXT,
        registry_mirrors TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )`)
	if err != nil {
		log.Printf("创建 docker_proxy 表失败: %v", err)
		return err
	}
	if err := ensureTableColumns("docker_proxy", []columnSpec{
		{Name: "enabled", AddColumnSQL: "enabled INTEGER DEFAULT 0", BackfillSQL: []string{"UPDATE docker_proxy SET enabled = 0 WHERE enabled IS NULL"}},
		{Name: "http_proxy", AddColumnSQL: "http_proxy TEXT"},
		{Name: "https_proxy", AddColumnSQL: "https_proxy TEXT"},
		{Name: "no_proxy", AddColumnSQL: "no_proxy TEXT"},
		{Name: "registry_mirrors", AddColumnSQL: "registry_mirrors TEXT"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE docker_proxy SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE docker_proxy SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}

	// 创建代理历史记录表
	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS proxy_history (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        enabled INTEGER DEFAULT 0,
        http_proxy TEXT,
        https_proxy TEXT,
        no_proxy TEXT,
        registry_mirrors TEXT,
        change_type TEXT,
        changed_at DATETIME DEFAULT CURRENT_TIMESTAMP
    )`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("proxy_history", []columnSpec{
		{Name: "enabled", AddColumnSQL: "enabled INTEGER DEFAULT 0", BackfillSQL: []string{"UPDATE proxy_history SET enabled = 0 WHERE enabled IS NULL"}},
		{Name: "http_proxy", AddColumnSQL: "http_proxy TEXT"},
		{Name: "https_proxy", AddColumnSQL: "https_proxy TEXT"},
		{Name: "no_proxy", AddColumnSQL: "no_proxy TEXT"},
		{Name: "registry_mirrors", AddColumnSQL: "registry_mirrors TEXT"},
		{Name: "change_type", AddColumnSQL: "change_type TEXT"},
		{Name: "changed_at", AddColumnSQL: "changed_at DATETIME", BackfillSQL: []string{"UPDATE proxy_history SET changed_at = CURRENT_TIMESTAMP WHERE changed_at IS NULL"}},
	}); err != nil {
		return err
	}

	// 创建导航项表
	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS navigation_items (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL,
        url TEXT,
        lan_url TEXT,
        wan_url TEXT,
        icon TEXT,
        category TEXT,
	        is_auto INTEGER DEFAULT 0,
	        is_deleted INTEGER DEFAULT 0,
	        hidden_reason TEXT DEFAULT '',
	        container_id TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `)
	if err != nil {
		return err
	}

	if err := ensureTableColumns("navigation_items", []columnSpec{
		{Name: "is_deleted", AddColumnSQL: "is_deleted INTEGER DEFAULT 0"},
		{Name: "hidden_reason", AddColumnSQL: "hidden_reason TEXT DEFAULT ''"},
		{Name: "icon_path", AddColumnSQL: "icon_path TEXT"},
		{Name: "ai_generated", AddColumnSQL: "ai_generated INTEGER DEFAULT 0"},
	}); err != nil {
		return err
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS compose_git_sources (
	        environment_id TEXT NOT NULL DEFAULT 'local',
	        project_name TEXT NOT NULL,
	        repo_url TEXT NOT NULL,
	        branch TEXT,
	        accelerator_url TEXT,
	        commit_hash TEXT,
	        compose_path TEXT,
	        synced_at DATETIME,
	        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	        PRIMARY KEY (environment_id, project_name)
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("compose_git_sources", []columnSpec{
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE compose_git_sources SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
		{Name: "repo_url", AddColumnSQL: "repo_url TEXT"},
		{Name: "branch", AddColumnSQL: "branch TEXT"},
		{Name: "accelerator_url", AddColumnSQL: "accelerator_url TEXT"},
		{Name: "commit_hash", AddColumnSQL: "commit_hash TEXT"},
		{Name: "compose_path", AddColumnSQL: "compose_path TEXT"},
		{Name: "synced_at", AddColumnSQL: "synced_at DATETIME"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME"},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME"},
	}); err != nil {
		return err
	}
	if err := ensureComposeGitSourceEnvironmentKey(); err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS compose_project_metadata (
			environment_id TEXT NOT NULL DEFAULT 'local',
			project_name TEXT NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (environment_id, project_name)
		);
		CREATE INDEX IF NOT EXISTS idx_compose_project_metadata_environment
		ON compose_project_metadata(environment_id, updated_at DESC);
	`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS compose_project_identities (
			environment_id TEXT NOT NULL DEFAULT 'local',
			relative_path TEXT NOT NULL,
			compose_project_name TEXT NOT NULL,
			source TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (environment_id, relative_path),
			UNIQUE (environment_id, compose_project_name)
		);
		CREATE INDEX IF NOT EXISTS idx_compose_project_identities_name
		ON compose_project_identities(environment_id, compose_project_name);
	`)
	if err != nil {
		return err
	}
	if err := createComposeHistoryTable(); err != nil {
		return err
	}

	if err := createEnvironmentAndDeploymentTables(); err != nil {
		return err
	}
	if err := createNotificationChannelTables(); err != nil {
		return err
	}

	// 创建全局设置表
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS global_settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS port_settings (
	        id INTEGER PRIMARY KEY AUTOINCREMENT,
	        range_start INTEGER NOT NULL,
	        range_end INTEGER NOT NULL,
	        protocol TEXT NOT NULL,
	        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("port_settings", []columnSpec{
		{Name: "range_start", AddColumnSQL: "range_start INTEGER"},
		{Name: "range_end", AddColumnSQL: "range_end INTEGER"},
		{Name: "protocol", AddColumnSQL: "protocol TEXT"},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE port_settings SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS port_notes (
	        port INTEGER NOT NULL,
	        type TEXT NOT NULL,
	        protocol TEXT NOT NULL,
	        note TEXT,
	        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	        PRIMARY KEY (port, type, protocol)
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("port_notes", []columnSpec{
		{Name: "note", AddColumnSQL: "note TEXT"},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE port_notes SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS port_reservations (
	        port INTEGER PRIMARY KEY,
	        reserved_by TEXT,
	        protocol TEXT,
	        type TEXT,
	        reserved_at DATETIME DEFAULT CURRENT_TIMESTAMP
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("port_reservations", []columnSpec{
		{Name: "reserved_by", AddColumnSQL: "reserved_by TEXT"},
		{Name: "protocol", AddColumnSQL: "protocol TEXT"},
		{Name: "type", AddColumnSQL: "type TEXT"},
		{Name: "reserved_at", AddColumnSQL: "reserved_at DATETIME", BackfillSQL: []string{"UPDATE port_reservations SET reserved_at = CURRENT_TIMESTAMP WHERE reserved_at IS NULL"}},
	}); err != nil {
		return err
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS tasks (
	        id TEXT PRIMARY KEY,
	        environment_id TEXT NOT NULL DEFAULT 'local',
	        type TEXT NOT NULL,
	        status TEXT NOT NULL,
	        result_json TEXT,
	        error TEXT,
	        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("tasks", []columnSpec{
		{Name: "request_digest", AddColumnSQL: "request_digest TEXT NOT NULL DEFAULT ''"},
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE tasks SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
		{Name: "type", AddColumnSQL: "type TEXT"},
		{Name: "status", AddColumnSQL: "status TEXT"},
		{Name: "result_json", AddColumnSQL: "result_json TEXT"},
		{Name: "error", AddColumnSQL: "error TEXT"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE tasks SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE tasks SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS task_logs (
	        id INTEGER PRIMARY KEY AUTOINCREMENT,
	        task_id TEXT NOT NULL,
		environment_id TEXT NOT NULL DEFAULT 'local',
	        seq INTEGER NOT NULL,
	        time DATETIME DEFAULT CURRENT_TIMESTAMP,
	        type TEXT,
	        message TEXT,
	        UNIQUE(task_id, seq)
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("task_logs", []columnSpec{
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE task_logs SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
		{Name: "task_id", AddColumnSQL: "task_id TEXT"},
		{Name: "seq", AddColumnSQL: "seq INTEGER"},
		{Name: "time", AddColumnSQL: "time DATETIME"},
		{Name: "type", AddColumnSQL: "type TEXT"},
		{Name: "message", AddColumnSQL: "message TEXT"},
	}); err != nil {
		return err
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_logs_task_seq ON task_logs(task_id, seq)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_environment_updated ON tasks(environment_id, updated_at DESC)`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_logs_environment_task_seq ON task_logs(environment_id, task_id, seq)`)

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS notifications (
	        id INTEGER PRIMARY KEY AUTOINCREMENT,
		environment_id TEXT NOT NULL DEFAULT 'local',
	        type TEXT,
	        message TEXT,
	        read INTEGER DEFAULT 0,
	        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("notifications", []columnSpec{
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE notifications SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
		{Name: "type", AddColumnSQL: "type TEXT"},
		{Name: "event_type", AddColumnSQL: "event_type TEXT DEFAULT 'general'", BackfillSQL: []string{"UPDATE notifications SET event_type = 'general' WHERE event_type IS NULL OR event_type = ''"}},
		{Name: "category", AddColumnSQL: "category TEXT DEFAULT 'system'", BackfillSQL: []string{"UPDATE notifications SET category = 'system' WHERE category IS NULL OR category = ''"}},
		{Name: "message", AddColumnSQL: "message TEXT"},
		{Name: "read", AddColumnSQL: "read INTEGER DEFAULT 0", BackfillSQL: []string{"UPDATE notifications SET read = 0 WHERE read IS NULL"}},
		{Name: "hidden", AddColumnSQL: "hidden INTEGER DEFAULT 0", BackfillSQL: []string{"UPDATE notifications SET hidden = 0 WHERE hidden IS NULL"}},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE notifications SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "hidden_at", AddColumnSQL: "hidden_at DATETIME"},
	}); err != nil {
		return err
	}
	if err := backfillNotificationCategories(); err != nil {
		return err
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_notifications_environment_created ON notifications(environment_id, id DESC)`)

	_, err = db.Exec(`
	    CREATE TABLE IF NOT EXISTS image_updates (
	        id INTEGER PRIMARY KEY AUTOINCREMENT,
	        repo_tag TEXT NOT NULL UNIQUE,
	        image_id TEXT,
	        local_digest TEXT,
	        remote_digest TEXT,
	        notified INTEGER DEFAULT 0,
	        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("image_updates", []columnSpec{
		{Name: "repo_tag", AddColumnSQL: "repo_tag TEXT"},
		{Name: "image_id", AddColumnSQL: "image_id TEXT"},
		{Name: "local_digest", AddColumnSQL: "local_digest TEXT"},
		{Name: "remote_digest", AddColumnSQL: "remote_digest TEXT"},
		{Name: "notified", AddColumnSQL: "notified INTEGER DEFAULT 0", BackfillSQL: []string{"UPDATE image_updates SET notified = 0 WHERE notified IS NULL"}},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE image_updates SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE image_updates SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}

	if err := ensureImageUpdatesNotifiedColumn(); err != nil {
		return err
	}

	// 定时任务调度规则表（scheduled_jobs）。注意与 tasks 表的区别：
	// tasks 表是"一次执行的临时记录"，scheduled_jobs 是"调度规则定义"，
	// 二者通过 last_task_id 软关联，不使用外键。
	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS scheduled_jobs (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
		environment_id TEXT NOT NULL DEFAULT 'local',
        name TEXT NOT NULL,
        task_type TEXT NOT NULL,
        cron_expr TEXT NOT NULL,
        target TEXT,
        payload_json TEXT,
        enabled INTEGER DEFAULT 1,
        last_run_at DATETIME,
        next_run_at DATETIME,
        last_task_id TEXT,
        last_status TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_enabled ON scheduled_jobs(enabled);
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("scheduled_jobs", []columnSpec{
		{Name: "environment_id", AddColumnSQL: "environment_id TEXT DEFAULT 'local'", BackfillSQL: []string{"UPDATE scheduled_jobs SET environment_id = 'local' WHERE environment_id IS NULL OR environment_id = ''"}},
		{Name: "name", AddColumnSQL: "name TEXT"},
		{Name: "task_type", AddColumnSQL: "task_type TEXT"},
		{Name: "cron_expr", AddColumnSQL: "cron_expr TEXT"},
		{Name: "target", AddColumnSQL: "target TEXT"},
		{Name: "payload_json", AddColumnSQL: "payload_json TEXT"},
		{Name: "enabled", AddColumnSQL: "enabled INTEGER DEFAULT 1", BackfillSQL: []string{"UPDATE scheduled_jobs SET enabled = 1 WHERE enabled IS NULL"}},
		{Name: "last_run_at", AddColumnSQL: "last_run_at DATETIME"},
		{Name: "next_run_at", AddColumnSQL: "next_run_at DATETIME"},
		{Name: "last_task_id", AddColumnSQL: "last_task_id TEXT"},
		{Name: "last_status", AddColumnSQL: "last_status TEXT"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE scheduled_jobs SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE scheduled_jobs SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_environment ON scheduled_jobs(environment_id, enabled, id ASC)`)

	_, err = db.Exec(`
    CREATE TABLE IF NOT EXISTS image_remote_digest_status (
	        repo_tag TEXT PRIMARY KEY,
	        fail_count INTEGER DEFAULT 0,
	        unavailable INTEGER DEFAULT 0,
	        next_check_at DATETIME,
	        last_error TEXT,
	        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	    );
	`)
	if err != nil {
		return err
	}
	if err := ensureTableColumns("image_remote_digest_status", []columnSpec{
		{Name: "repo_tag", AddColumnSQL: "repo_tag TEXT"},
		{Name: "fail_count", AddColumnSQL: "fail_count INTEGER DEFAULT 0", BackfillSQL: []string{"UPDATE image_remote_digest_status SET fail_count = 0 WHERE fail_count IS NULL"}},
		{Name: "unavailable", AddColumnSQL: "unavailable INTEGER DEFAULT 0", BackfillSQL: []string{"UPDATE image_remote_digest_status SET unavailable = 0 WHERE unavailable IS NULL"}},
		{Name: "next_check_at", AddColumnSQL: "next_check_at DATETIME"},
		{Name: "last_error", AddColumnSQL: "last_error TEXT"},
		{Name: "created_at", AddColumnSQL: "created_at DATETIME", BackfillSQL: []string{"UPDATE image_remote_digest_status SET created_at = CURRENT_TIMESTAMP WHERE created_at IS NULL"}},
		{Name: "updated_at", AddColumnSQL: "updated_at DATETIME", BackfillSQL: []string{"UPDATE image_remote_digest_status SET updated_at = CURRENT_TIMESTAMP WHERE updated_at IS NULL"}},
	}); err != nil {
		return err
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ai_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		scope TEXT,
		level TEXT,
		message TEXT,
		details TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}
	if err := createEditionDatabaseTables(); err != nil {
		return err
	}

	if err := cleanupLegacySchema(); err != nil {
		return err
	}

	return nil
}

func cleanupLegacySchema() error {
	if _, err := db.Exec(`DELETE FROM global_settings WHERE key = 'server_url'`); err != nil {
		return err
	}

	for _, table := range []string{"deployments", "applications"} {
		if err := dropTableIfEmpty(table); err != nil {
			return err
		}
	}

	return nil
}

func dropTableIfEmpty(table string) error {
	if !isValidIdentifier(table) {
		return fmt.Errorf("非法表名: %s", table)
	}

	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		log.Printf("[数据库] 发现废弃表 %s 仍有 %d 条数据，暂不自动删除", table, count)
		return nil
	}

	if _, err := db.Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
		return err
	}
	log.Printf("[数据库] 已清理空的废弃表: %s", table)
	return nil
}

// ensureImageUpdatesNotifiedColumn 确保 image_updates 表包含 notified 列，兼容旧数据库。
func ensureImageUpdatesNotifiedColumn() error {
	return ensureTableColumns("image_updates", []columnSpec{
		{
			Name:         "notified",
			AddColumnSQL: "notified INTEGER DEFAULT 0",
			BackfillSQL:  []string{`UPDATE image_updates SET notified = 1`},
		},
	})
}

type Notification struct {
	ID            int64  `json:"id"`
	EnvironmentID string `json:"environmentId"`
	Type          string `json:"type"`
	EventType     string `json:"eventType,omitempty"`
	Category      string `json:"category"`
	Message       string `json:"message"`
	CreatedAt     string `json:"created_at"`
	Read          bool   `json:"read"`
	Hidden        bool   `json:"hidden"`
	HiddenAt      string `json:"hidden_at,omitempty"`
}

type NotificationSummary struct {
	TotalCount    int    `json:"total_count"`
	UnreadCount   int    `json:"unread_count"`
	LatestID      int64  `json:"latest_id"`
	LatestCreated string `json:"latest_created_at"`
}

type ImageUpdate struct {
	ID           int64  `json:"id"`
	RepoTag      string `json:"repo_tag"`
	ImageID      string `json:"image_id"`
	LocalDigest  string `json:"local_digest"`
	RemoteDigest string `json:"remote_digest"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Notified     bool   `json:"notified"`
}

type ImageRemoteDigestStatus struct {
	RepoTag     string `json:"repoTag"`
	FailCount   int    `json:"failCount"`
	Unavailable bool   `json:"unavailable"`
	NextCheckAt string `json:"nextCheckAt"`
	LastError   string `json:"lastError"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

func GetImageRemoteDigestStatus(repoTag string) (ImageRemoteDigestStatus, error) {
	var s ImageRemoteDigestStatus
	repoTag = strings.TrimSpace(repoTag)
	if repoTag == "" {
		return s, nil
	}

	var unavailable int
	err := db.QueryRow(`
	    SELECT repo_tag, fail_count, unavailable, COALESCE(next_check_at, ''), COALESCE(last_error, ''), COALESCE(created_at, ''), COALESCE(updated_at, '')
	    FROM image_remote_digest_status
	    WHERE repo_tag = ?
	`, repoTag).Scan(&s.RepoTag, &s.FailCount, &unavailable, &s.NextCheckAt, &s.LastError, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return ImageRemoteDigestStatus{RepoTag: repoTag}, nil
	}
	if err != nil {
		return s, err
	}
	s.Unavailable = unavailable == 1
	return s, nil
}

func ResetImageRemoteDigestStatus(repoTag string) error {
	repoTag = strings.TrimSpace(repoTag)
	if repoTag == "" {
		return nil
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err := db.Exec(`
	    INSERT INTO image_remote_digest_status (repo_tag, fail_count, unavailable, next_check_at, last_error, created_at, updated_at)
	    VALUES (?, 0, 0, NULL, '', ?, ?)
	    ON CONFLICT(repo_tag) DO UPDATE SET
	      fail_count = 0,
	      unavailable = 0,
	      next_check_at = NULL,
	      last_error = '',
	      updated_at = excluded.updated_at
	`, repoTag, now, now)
	return err
}

func RecordImageRemoteDigestFailure(repoTag string, errMsg string, firstBackoff time.Duration, secondBackoff time.Duration, maxFail int) (ImageRemoteDigestStatus, error) {
	repoTag = strings.TrimSpace(repoTag)
	if repoTag == "" {
		return ImageRemoteDigestStatus{}, nil
	}

	errMsg = logging.RedactText(strings.TrimSpace(errMsg))
	if len(errMsg) > 800 {
		errMsg = errMsg[:800]
	}

	if maxFail <= 0 {
		maxFail = 3
	}
	if firstBackoff <= 0 {
		firstBackoff = 24 * time.Hour
	}
	if secondBackoff <= 0 {
		secondBackoff = 48 * time.Hour
	}

	existing, err := GetImageRemoteDigestStatus(repoTag)
	if err != nil {
		return ImageRemoteDigestStatus{}, err
	}

	newFail := existing.FailCount + 1
	if newFail > maxFail {
		newFail = maxFail
	}
	unavailable := newFail >= maxFail

	nowTime := time.Now().In(chinaLocation)
	now := nowTime.Format(time.RFC3339)

	nextCheckAt := ""
	if !unavailable {
		if newFail == 1 {
			nextCheckAt = nowTime.Add(firstBackoff).Format(time.RFC3339)
		} else {
			nextCheckAt = nowTime.Add(secondBackoff).Format(time.RFC3339)
		}
	}

	nextCheckAtSQL := sql.NullString{Valid: false}
	if nextCheckAt != "" {
		nextCheckAtSQL = sql.NullString{String: nextCheckAt, Valid: true}
	}

	_, err = db.Exec(`
	    INSERT INTO image_remote_digest_status (repo_tag, fail_count, unavailable, next_check_at, last_error, created_at, updated_at)
	    VALUES (?, ?, ?, ?, ?, ?, ?)
	    ON CONFLICT(repo_tag) DO UPDATE SET
	      fail_count = excluded.fail_count,
	      unavailable = excluded.unavailable,
	      next_check_at = excluded.next_check_at,
	      last_error = excluded.last_error,
	      updated_at = excluded.updated_at
	`, repoTag, newFail, boolToInt(unavailable), nextCheckAtSQL, errMsg, now, now)
	if err != nil {
		return ImageRemoteDigestStatus{}, err
	}
	return GetImageRemoteDigestStatus(repoTag)
}

func ParseSQLiteTime(v string) (time.Time, bool) {
	return ParseStoredTime(v)
}

// GetAllUnavailableImageRemoteDigests 获取所有远程不可用的镜像列表
func GetAllUnavailableImageRemoteDigests() ([]string, error) {
	rows, err := db.Query(`SELECT repo_tag FROM image_remote_digest_status WHERE unavailable = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []string
	for rows.Next() {
		var repoTag string
		if err := rows.Scan(&repoTag); err != nil {
			continue
		}
		list = append(list, strings.TrimSpace(repoTag))
	}
	return list, nil
}

// TaskRecord 表示后台任务的数据库记录（用于断线续看/进度查询）。
type TaskRecord struct {
	ID            string `json:"id"`
	EnvironmentID string `json:"environmentId"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	ResultJSON    string `json:"result_json"`
	Error         string `json:"error"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// TaskLogRecord 表示任务日志的数据库记录（seq 用作 SSE 的 id/游标）。
type TaskLogRecord struct {
	TaskID        string `json:"task_id"`
	EnvironmentID string `json:"environmentId"`
	Seq           int64  `json:"seq"`
	Time          string `json:"time"`
	Type          string `json:"type"`
	Message       string `json:"message"`
}

func normalizeNotificationCategory(category string) string {
	category = strings.TrimSpace(strings.ToLower(category))
	switch category {
	case "deploy_task", "git_task", "navigation_task", "volume_backup_task", "app_protection_task", "system":
		return category
	default:
		return "system"
	}
}

func normalizeNotificationEventType(eventType string) string {
	eventType = strings.TrimSpace(strings.ToLower(eventType))
	if eventType == "" {
		return "general"
	}
	for _, char := range eventType {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return "general"
		}
	}
	return eventType
}

// SetNotificationEmitter registers the process-local external notification
// bridge. Delivery is deliberately asynchronous and never changes the result
// of the business operation that created the in-app notification.
func SetNotificationEmitter(fn func(Notification)) {
	notificationEmitter.Lock()
	notificationEmitter.fn = fn
	notificationEmitter.Unlock()
}

func emitNotification(notification Notification) {
	notificationEmitter.RLock()
	fn := notificationEmitter.fn
	notificationEmitter.RUnlock()
	if fn == nil {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		fn(notification)
	}()
}

func inferNotificationCategory(message string) string {
	message = strings.TrimSpace(message)
	switch {
	case strings.Contains(message, "应用部署") ||
		strings.Contains(message, "Compose 项目") && (strings.Contains(message, "部署") ||
			strings.Contains(message, "构建") ||
			strings.Contains(message, "更新") ||
			strings.Contains(message, "启动") ||
			strings.Contains(message, "停止") ||
			strings.Contains(message, "重启")):
		return "deploy_task"
	case strings.Contains(message, "GitHub 应用") ||
		strings.Contains(message, " Git ") ||
		strings.Contains(message, "Git 导入") ||
		strings.Contains(message, "Git 同步"):
		return "git_task"
	case strings.Contains(message, "AI 导航识别"):
		return "navigation_task"
	case strings.Contains(message, "应用备份") ||
		strings.Contains(message, "应用恢复"):
		return "app_protection_task"
	case strings.Contains(message, "卷备份"):
		return "volume_backup_task"
	default:
		return "system"
	}
}

func backfillNotificationCategories() error {
	rows, err := db.Query(`SELECT id, COALESCE(message, '') FROM notifications WHERE category IS NULL OR category = '' OR category = 'system'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type updateRow struct {
		id       int64
		category string
	}
	updates := []updateRow{}
	for rows.Next() {
		var id int64
		var message string
		if err := rows.Scan(&id, &message); err != nil {
			return err
		}
		category := inferNotificationCategory(message)
		if category != "system" {
			updates = append(updates, updateRow{id: id, category: category})
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range updates {
		if _, err := db.Exec(`UPDATE notifications SET category = ? WHERE id = ?`, item.category, item.id); err != nil {
			return err
		}
	}
	return nil
}

func notificationCategoryAllowed(category string) bool {
	category = normalizeNotificationCategory(category)
	var raw string
	err := db.QueryRow("SELECT value FROM global_settings WHERE key = ?", "notification_enabled_categories").Scan(&raw)
	if err != nil || strings.TrimSpace(raw) == "" {
		return true
	}
	var values []string
	if json.Unmarshal([]byte(raw), &values) != nil {
		values = strings.Split(raw, ",")
	}
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if normalizeNotificationCategory(value) == category {
			return true
		}
	}
	return false
}

func SaveNotification(n *Notification) error {
	return SaveNotificationInEnvironment(LocalEnvironmentID, n)
}

func SaveNotificationInEnvironment(environmentID string, n *Notification) error {
	if n == nil {
		return nil
	}
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	if n.Message == "" {
		return nil
	}
	n.Message = logging.RedactText(secrets.RedactString(n.Message))
	n.Category = normalizeNotificationCategory(n.Category)
	n.EventType = normalizeNotificationEventType(n.EventType)
	if !notificationCategoryAllowed(n.Category) {
		return nil
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	read := boolToInt(n.Read)
	res, err := db.Exec(`
	    INSERT INTO notifications (environment_id, type, event_type, category, message, read, created_at)
	    VALUES (?, ?, ?, ?, ?, ?, ?)
	`, environmentID, n.Type, n.EventType, n.Category, n.Message, read, now)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err == nil {
		n.ID = id
		n.EnvironmentID = environmentID
		n.CreatedAt = now
		emitNotification(*n)
	}
	return nil
}

func GetNotifications(limit int) ([]Notification, error) {
	return GetNotificationsInEnvironment(LocalEnvironmentID, limit)
}

func GetNotificationsInEnvironment(environmentID string, limit int) ([]Notification, error) {
	return GetNotificationsPageInEnvironment(environmentID, limit, 0)
}

func GetNotificationsPage(limit int, offset int) ([]Notification, error) {
	return GetNotificationsPageInEnvironment(LocalEnvironmentID, limit, offset)
}

func GetNotificationsPageInEnvironment(environmentID string, limit int, offset int) ([]Notification, error) {
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := db.Query(`
	    SELECT id, environment_id, type, COALESCE(event_type, 'general'), COALESCE(category, 'system'), message, created_at, read, hidden, COALESCE(hidden_at, '')
	    FROM notifications
	    WHERE environment_id = ? AND hidden = 0
	    ORDER BY id DESC
	    LIMIT ?
	    OFFSET ?
	`, environmentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func GetNotificationsBeforeID(limit int, beforeID int64) ([]Notification, error) {
	return GetNotificationsBeforeIDInEnvironment(LocalEnvironmentID, limit, beforeID)
}

func GetNotificationsBeforeIDInEnvironment(environmentID string, limit int, beforeID int64) ([]Notification, error) {
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if beforeID <= 0 {
		return GetNotificationsPageInEnvironment(environmentID, limit, 0)
	}

	rows, err := db.Query(`
	    SELECT id, environment_id, type, COALESCE(event_type, 'general'), COALESCE(category, 'system'), message, created_at, read, hidden, COALESCE(hidden_at, '')
	    FROM notifications
	    WHERE environment_id = ? AND hidden = 0 AND id < ?
	    ORDER BY id DESC
	    LIMIT ?
	`, environmentID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func scanNotifications(rows *sql.Rows) ([]Notification, error) {
	var list []Notification
	for rows.Next() {
		var n Notification
		var readInt int
		var hiddenInt int
		if err := rows.Scan(&n.ID, &n.EnvironmentID, &n.Type, &n.EventType, &n.Category, &n.Message, &n.CreatedAt, &readInt, &hiddenInt, &n.HiddenAt); err != nil {
			return nil, err
		}
		n.Category = normalizeNotificationCategory(n.Category)
		n.EventType = normalizeNotificationEventType(n.EventType)
		n.Read = readInt == 1
		n.Hidden = hiddenInt == 1
		list = append(list, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

func DeleteNotification(id int64) error {
	return DeleteNotificationInEnvironment(LocalEnvironmentID, id)
}

func DeleteNotificationInEnvironment(environmentID string, id int64) error {
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE notifications SET hidden = 1, hidden_at = CURRENT_TIMESTAMP WHERE id = ? AND environment_id = ?`, id, environmentID)
	return err
}

func ClearAllNotifications() error {
	return ClearAllNotificationsInEnvironment(LocalEnvironmentID)
}

func ClearAllNotificationsInEnvironment(environmentID string) error {
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE notifications SET hidden = 1, hidden_at = CURRENT_TIMESTAMP WHERE environment_id = ? AND hidden = 0`, environmentID)
	return err
}

func MarkAllNotificationsRead() error {
	return MarkAllNotificationsReadInEnvironment(LocalEnvironmentID)
}

func MarkAllNotificationsReadInEnvironment(environmentID string) error {
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE notifications SET read = 1 WHERE environment_id = ? AND read = 0 AND hidden = 0`, environmentID)
	return err
}

func GetNotificationSummary() (NotificationSummary, error) {
	return GetNotificationSummaryInEnvironment(LocalEnvironmentID)
}

func GetNotificationSummaryInEnvironment(environmentID string) (NotificationSummary, error) {
	var summary NotificationSummary
	var err error
	environmentID, err = normalizeNotificationEnvironmentID(environmentID)
	if err != nil {
		return summary, err
	}
	err = db.QueryRow(`
	    SELECT
	      COUNT(*),
	      COALESCE(SUM(CASE WHEN read = 0 THEN 1 ELSE 0 END), 0),
	      COALESCE(MAX(id), 0),
	      COALESCE(MAX(created_at), '')
	    FROM notifications
	    WHERE environment_id = ? AND hidden = 0
	`, environmentID).Scan(&summary.TotalCount, &summary.UnreadCount, &summary.LatestID, &summary.LatestCreated)
	return summary, err
}

func normalizeNotificationEnvironmentID(environmentID string) (string, error) {
	environmentID = strings.TrimSpace(environmentID)
	if environmentID == "" {
		environmentID = LocalEnvironmentID
	}
	if !validEnvironmentID(environmentID) {
		return "", fmt.Errorf("环境 ID 不合法")
	}
	if _, err := GetEnvironment(environmentID); err != nil {
		return "", err
	}
	return environmentID, nil
}

func ClearImageUpdates() error {
	_, err := db.Exec(`DELETE FROM image_updates`)
	return err
}

func DeleteImageUpdateByRepoTag(repoTag string) error {
	if repoTag == "" {
		return nil
	}
	_, err := db.Exec(`DELETE FROM image_updates WHERE repo_tag = ?`, repoTag)
	return err
}

func SaveImageUpdate(u *ImageUpdate) error {
	if u == nil {
		return nil
	}
	if u.RepoTag == "" {
		return nil
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	notified := boolToInt(u.Notified)
	_, err := db.Exec(`
	    INSERT INTO image_updates (repo_tag, image_id, local_digest, remote_digest, notified, created_at, updated_at)
	    VALUES (?, ?, ?, ?, ?, ?, ?)
	    ON CONFLICT(repo_tag) DO UPDATE SET
	      image_id = excluded.image_id,
	      local_digest = excluded.local_digest,
	      remote_digest = excluded.remote_digest,
	      notified = excluded.notified,
	      updated_at = excluded.updated_at
	`, u.RepoTag, u.ImageID, u.LocalDigest, u.RemoteDigest, notified, now, now)
	if err != nil {
		return err
	}
	_ = db.QueryRow(`
	    SELECT id, created_at, updated_at, notified
	    FROM image_updates
	    WHERE repo_tag = ?
	`, u.RepoTag).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt, &notified)
	u.Notified = notified == 1
	return nil
}

func GetAllImageUpdates() ([]ImageUpdate, error) {
	rows, err := db.Query(`
	    SELECT id, repo_tag, image_id, local_digest, remote_digest, created_at, updated_at, notified
	    FROM image_updates
	    ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ImageUpdate
	for rows.Next() {
		var u ImageUpdate
		var notified int
		if err := rows.Scan(&u.ID, &u.RepoTag, &u.ImageID, &u.LocalDigest, &u.RemoteDigest, &u.CreatedAt, &u.UpdatedAt, &notified); err != nil {
			return nil, err
		}
		u.Notified = notified == 1
		list = append(list, u)
	}

	return list, nil
}

func GetUnnotifiedImageUpdates() ([]ImageUpdate, error) {
	rows, err := db.Query(`
	    SELECT id, repo_tag, image_id, local_digest, remote_digest, created_at, updated_at, notified
	    FROM image_updates
	    WHERE notified = 0
	    ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ImageUpdate
	for rows.Next() {
		var u ImageUpdate
		var notified int
		if err := rows.Scan(&u.ID, &u.RepoTag, &u.ImageID, &u.LocalDigest, &u.RemoteDigest, &u.CreatedAt, &u.UpdatedAt, &notified); err != nil {
			return nil, err
		}
		u.Notified = notified == 1
		list = append(list, u)
	}

	return list, nil
}

func MarkImageUpdatesNotifiedByRepoTags(repoTags []string) error {
	if len(repoTags) == 0 {
		return nil
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	placeholders := make([]string, 0, len(repoTags))
	args := make([]any, 0, len(repoTags)+1)
	args = append(args, now)
	for _, t := range repoTags {
		placeholders = append(placeholders, "?")
		args = append(args, t)
	}
	query := `UPDATE image_updates SET notified = 1, updated_at = ? WHERE repo_tag IN (` + strings.Join(placeholders, ",") + `) AND notified = 0`
	_, err := db.Exec(query, args...)
	return err
}

// initAdminUser 初始化管理员账户
func initAdminUser() error {
	var count int
	// The administrator can rename their account; only bootstrap an empty table.
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		adminPassword := os.Getenv("ADMIN_PASSWORD")
		if adminPassword == "" {
			if os.Getenv("GIN_MODE") == "release" {
				log.Fatal("[FATAL] ADMIN_PASSWORD 环境变量必须在生产环境设置")
			}
			adminPassword = "123456" // 仅开发环境使用
			log.Println("[WARNING] ADMIN_PASSWORD 未设置，开发环境将使用默认凭据")
		}

		// 使用 bcrypt 哈希存储管理员密码
		hash, herr := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if herr != nil {
			return herr
		}
		_, err = db.Exec("INSERT INTO users (username, password) VALUES (?, ?)", "admin", string(hash))
		if err != nil {
			return err
		}
		log.Println("管理员账户已创建 (username: admin)")
	}
	return nil
}

func maybeResetAdminPassword() error {
	force := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_FORCE_RESET")))
	if force != "1" && force != "true" {
		return nil
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "123456"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	result, err := db.Exec(adminPasswordUpdateSQL, string(hash))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("administrator account is missing or ambiguous")
	}
	return nil
}
func GetDB() *sql.DB {
	return db
}

// SnapshotDB writes a self-contained consistent copy of the live database to
// destination using VACUUM INTO. Unlike copying the file, VACUUM INTO folds in
// WAL content, so the result reflects the latest committed state and stays
// consistent even while the application keeps writing to the live database.
// The passed context lets a slow snapshot be cancelled with the task that
// requested it.
func SnapshotDB(ctx context.Context, destination string) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	// VACUUM INTO 的文件名必须是 SQL 字面量，不能作为绑定参数，这里手动转义单引号。
	escaped := strings.ReplaceAll(destination, "'", "''")
	_, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'")
	return err
}

func Close() error {
	if db != nil {
		err := db.Close()
		db = nil
		return err
	}
	return nil
}
