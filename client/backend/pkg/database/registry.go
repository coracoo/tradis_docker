package database

import (
	"database/sql"
	"dockerpanel/backend/pkg/logging"
	"fmt"
	"time"
)

type Registry struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"` // 改为 string 类型
	UpdatedAt string `json:"updated_at"` // 改为 string 类型
}

// 添加清除注册表的函数
func ClearRegistries() error {
	_, err := db.Exec("DELETE FROM registries")
	return err
}

// 修改 SaveRegistry 函数，添加必填字段验证
// SaveRegistry 保存注册表配置
func SaveRegistry(registry *Registry) error {
	logging.Debug("registry persistence started", "name", registry.Name)

	// 检查数据库连接
	if db == nil {
		return fmt.Errorf("数据库连接未初始化")
	}

	// 检查是否已存在相同 URL 的注册表
	var id int64
	err := db.QueryRow("SELECT id FROM registries WHERE url = ?", registry.URL).Scan(&id)

	now := time.Now().In(chinaLocation).Format(time.RFC3339)

	if err == nil {
		// 更新现有注册表
		logging.Debug("updating existing registry", "name", registry.Name, "id", id)
		_, err = db.Exec(`
            UPDATE registries 
            SET name = ?, username = ?, password = ?, is_default = ?, updated_at = ? 
            WHERE id = ?
        `, registry.Name, registry.Username, registry.Password,
			boolToInt(registry.IsDefault), now, id)
		return err
	} else if err == sql.ErrNoRows {
		// 插入新注册表
		logging.Debug("inserting new registry", "name", registry.Name)

		// 确保 URL 不为空
		if registry.URL == "" {
			logging.Warn("registry address is empty", "name", registry.Name)
			return fmt.Errorf("注册表 URL 不能为空")
		}

		var result sql.Result
		result, err = db.Exec(`
            INSERT INTO registries 
            (name, url, username, password, is_default, created_at, updated_at) 
            VALUES (?, ?, ?, ?, ?, ?, ?)
        `, registry.Name, registry.URL, registry.Username, registry.Password,
			boolToInt(registry.IsDefault), now, now)

		if err != nil {
			logging.Error("registry insertion failed", "name", registry.Name, "error", err)
			return err
		}

		// 获取新插入的 ID
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}

		logging.Debug("registry persisted", "name", registry.Name, "id", id)
		return nil
	}

	return err
}

// GetAllRegistries 获取所有注册表配置
func GetAllRegistries() (map[string]*Registry, error) {
	rows, err := db.Query(`
        SELECT id, name, url, username, password, is_default, created_at, updated_at 
        FROM registries
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	registries := make(map[string]*Registry)
	var count int

	for rows.Next() {
		var r Registry
		var isDefault int
		var createdAt, updatedAt string

		err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.Username, &r.Password, &isDefault, &createdAt, &updatedAt)
		if err != nil {
			logging.Warn("registry row scan failed", "error", err)
			return nil, err
		}

		r.IsDefault = isDefault == 1

		logging.Debug("registry loaded", "id", r.ID, "name", r.Name)

		// 确保 URL 不为空
		if r.URL != "" {
			registries[r.URL] = &r
			count++
		} else {
			logging.Warn("registry with empty address skipped", "id", r.ID, "name", r.Name)
		}
	}

	logging.Debug("registry configuration loaded", "count", count)

	// 确保 Docker Hub 存在
	if _, ok := registries["docker.io"]; !ok {
		logging.Debug("default Docker Hub registry added")
		registries["docker.io"] = &Registry{
			Name:      "Docker Hub",
			URL:       "docker.io",
			IsDefault: true,
		}
	}

	for k, v := range registries {
		logging.Debug("registry configuration available", "key", k, "name", v.Name)
	}

	return registries, nil
}
