package database

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"
)

// DockerProxy 结构体表示 Docker 代理配置
type DockerProxy struct {
	ID              int64  `json:"id"`
	Enabled         bool   `json:"enabled"`
	HTTPProxy       string `json:"http_proxy"`
	HTTPSProxy      string `json:"https_proxy"`
	NoProxy         string `json:"no_proxy"`
	RegistryMirrors string `json:"registry_mirrors"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// GetDockerProxy 获取 Docker 代理配置
func GetDockerProxy() (*DockerProxy, error) {
	var proxy DockerProxy

	// 查询最新的配置
	row := db.QueryRow(`
        SELECT id, enabled, http_proxy, https_proxy, no_proxy, registry_mirrors, 
               created_at, updated_at 
        FROM docker_proxy 
        ORDER BY id DESC LIMIT 1
    `)

	var enabled int
	err := row.Scan(
		&proxy.ID,
		&enabled,
		&proxy.HTTPProxy,
		&proxy.HTTPSProxy,
		&proxy.NoProxy,
		&proxy.RegistryMirrors,
		&proxy.CreatedAt,
		&proxy.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			// 无记录时返回空配置，不写入数据库
			return &DockerProxy{
				Enabled:         false,
				HTTPProxy:       "",
				HTTPSProxy:      "",
				NoProxy:         "",
				RegistryMirrors: "",
			}, nil
		}
		return nil, err
	}

	proxy.Enabled = enabled == 1
	return &proxy, nil
}

// SaveDockerProxy 保存 Docker 代理配置。
// 通过事务内的 UPDATE-then-INSERT（依据受影响行数判断）保证原子性，
// 避免并发调用下两个请求都读到 count=0 而双双 INSERT，或都读到旧行而互相覆盖。
func SaveDockerProxy(proxy *DockerProxy) error {
	var enabled int
	if proxy.Enabled {
		enabled = 1
	}

	now := time.Now().In(chinaLocation).Format(time.RFC3339)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // 提交后回滚为空操作，安全

	// 先尝试更新最新的一行（语义上只保留一份配置，参考 GetDockerProxy 的读取逻辑）
	res, err := tx.Exec(`
        UPDATE docker_proxy
        SET enabled = ?, http_proxy = ?, https_proxy = ?, no_proxy = ?,
            registry_mirrors = ?, updated_at = ?
        WHERE id = (SELECT id FROM docker_proxy ORDER BY id DESC LIMIT 1)
    `, enabled, proxy.HTTPProxy, proxy.HTTPSProxy, proxy.NoProxy,
		proxy.RegistryMirrors, now)
	if err != nil {
		return err
	}

	// 若没有命中任何行，说明表为空，则插入新配置
	if affected, _ := res.RowsAffected(); affected == 0 {
		if _, err := tx.Exec(`
            INSERT INTO docker_proxy
            (enabled, http_proxy, https_proxy, no_proxy, registry_mirrors, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?, ?, ?)
        `, enabled, proxy.HTTPProxy, proxy.HTTPSProxy, proxy.NoProxy,
			proxy.RegistryMirrors, now, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteDockerProxy 删除当前 Docker 代理配置
func DeleteDockerProxy() error {
	_, err := db.Exec(`DELETE FROM docker_proxy`)
	return err
}

// MarshalRegistryMirrors 将镜像加速器列表转换为 JSON 字符串
func MarshalRegistryMirrors(mirrors []string) string {
	if len(mirrors) == 0 {
		return ""
	}

	data, err := json.Marshal(mirrors)
	if err != nil {
		log.Printf("序列化镜像加速器列表失败: %v", err)
		return ""
	}

	return string(data)
}

// ProxyHistory 记录代理变更历史
type ProxyHistory struct {
	ID              int64  `json:"id"`
	Enabled         bool   `json:"enabled"`
	HTTPProxy       string `json:"http_proxy"`
	HTTPSProxy      string `json:"https_proxy"`
	NoProxy         string `json:"no_proxy"`
	RegistryMirrors string `json:"registry_mirrors"`
	ChangeType      string `json:"change_type"`
	ChangedAt       string `json:"changed_at"`
}

// SaveProxyHistory 写入代理历史记录
func SaveProxyHistory(ph *ProxyHistory) error {
	var enabled int
	if ph.Enabled {
		enabled = 1
	}
	now := time.Now().In(chinaLocation).Format(time.RFC3339)
	_, err := db.Exec(`
        INSERT INTO proxy_history (enabled, http_proxy, https_proxy, no_proxy, registry_mirrors, change_type, changed_at)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, enabled, ph.HTTPProxy, ph.HTTPSProxy, ph.NoProxy, ph.RegistryMirrors, ph.ChangeType, now)
	return err
}

// GetProxyHistory 获取最近的代理历史记录
func GetProxyHistory(limit int) ([]ProxyHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(`
        SELECT id, enabled, http_proxy, https_proxy, no_proxy, registry_mirrors, change_type, changed_at
        FROM proxy_history
        ORDER BY id DESC
        LIMIT ?
    `, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ProxyHistory
	for rows.Next() {
		var ph ProxyHistory
		var enabled int
		if err := rows.Scan(&ph.ID, &enabled, &ph.HTTPProxy, &ph.HTTPSProxy, &ph.NoProxy, &ph.RegistryMirrors, &ph.ChangeType, &ph.ChangedAt); err != nil {
			return nil, err
		}
		ph.Enabled = enabled == 1
		list = append(list, ph)
	}
	return list, nil
}
