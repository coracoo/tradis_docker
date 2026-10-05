package database

import (
	"strings"
	"time"
)

// 项目约定：所有落库的 naive 时间串（形如 "2006-01-02 15:04:05"）一律视为
// 中国时间（Asia/Shanghai, UTC+8）。ChinaLocation 显式固定时区，不依赖容器
// 的 TZ 环境变量，保证无论部署在哪种时区的环境里，写入和解析都一致。
var chinaLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil || loc == nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return loc
}()

const sqliteStampLayout = "2006-01-02 15:04:05"

// NowStamp 返回当前中国时间的 RFC3339 串（带 +08:00 偏移），落库后 go-sqlite3
// 会保留偏移、读回正确的瞬间，前端按中国时间展示。绝不再用 naive 串（会被驱动当 UTC）。
func NowStamp() string {
	return time.Now().In(chinaLocation).Format(time.RFC3339)
}

// FormatStamp 把任意 time.Time 转成中国时间 RFC3339 串；调用方无论传入 UTC 还是
// 本地时间，落库结果都是带偏移的明确瞬间。
func FormatStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(chinaLocation).Format(time.RFC3339)
}

// ParseStoredTime 解析落库时间串：优先按 RFC3339（带时区偏移，明确）解析；
// 否则按 naive 串视为中国时间。返回的 time.Time 统一在中国时区下。
func ParseStoredTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.In(chinaLocation), true
	}
	if t, err := time.ParseInLocation(sqliteStampLayout, v, chinaLocation); err == nil {
		return t, true
	}
	return time.Time{}, false
}
