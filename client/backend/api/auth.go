package api

import (
	"database/sql"
	"dockerpanel/backend/pkg/config"
	"dockerpanel/backend/pkg/database"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/golang-jwt/jwt/v5"
	"github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var jwtSecret []byte

type loginRateState struct {
	windowStart time.Time
	count       int
}

var (
	loginRateMu     sync.Mutex
	loginRateByIP   = map[string]loginRateState{}
	loginRateLimit  = 20
	loginRateWindow = time.Minute
)

func init() {
	secret := os.Getenv("JWT_SECRET")

	if secret == "" {
		// 生产环境必须设置JWT_SECRET
		if os.Getenv("GIN_MODE") == "release" {
			log.Fatal("[FATAL] JWT_SECRET environment variable must be set in production mode")
		}

		// 开发环境使用警告但仍允许启动（为了向后兼容）
		log.Println("[WARNING] JWT_SECRET not set, using temporary key. DO NOT USE IN PRODUCTION!")
		secret = "dev_temporary_key_change_me_" + time.Now().Format("20060102")
	}

	if len(secret) < 32 {
		log.Println("[WARNING] JWT_SECRET should be at least 32 characters for security")
	}

	jwtSecret = []byte(secret)
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type UpdatePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

type UpdateUsernameRequest struct {
	NewUsername     string `json:"newUsername" binding:"required"`
	CurrentPassword string `json:"currentPassword" binding:"required"`
}

// isSecureConnection 判断当前请求是否通过 HTTPS（含反向代理场景）
func isSecureConnection(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

func RegisterAuthRoutes(r *gin.Engine) {
	r.POST("/api/auth/login", loginRateLimitMiddleware(), login)

	authGroup := r.Group("/api/auth")
	authGroup.Use(AuthMiddleware())
	{
		authGroup.POST("/logout", logout)
		authGroup.POST("/change-password", changePassword)
		authGroup.POST("/change-username", changeUsername)
		authGroup.GET("/me", getCurrentUser)
	}
}

func loginRateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		now := time.Now()
		ip := strings.TrimSpace(c.ClientIP())

		loginRateMu.Lock()
		state := loginRateByIP[ip]
		if state.windowStart.IsZero() || now.Sub(state.windowStart) >= loginRateWindow {
			state = loginRateState{windowStart: now}
		}
		if len(loginRateByIP) > 1024 {
			for key, item := range loginRateByIP {
				if now.Sub(item.windowStart) >= loginRateWindow {
					delete(loginRateByIP, key)
				}
			}
		}
		state.count++
		loginRateByIP[ip] = state
		limited := state.count > loginRateLimit
		loginRateMu.Unlock()

		if limited {
			respondError(c, http.StatusTooManyRequests, "Too many login attempts, please try again later", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func validateNewPassword(password string) error {
	return database.ValidateAdminPassword(password)
}

func login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return
	}

	db := database.GetDB()
	var storedPassword string
	var userID, tokenVersion int
	err := db.QueryRowContext(c.Request.Context(), "SELECT id, password, token_version FROM users WHERE username = ?", req.Username).Scan(&userID, &storedPassword, &tokenVersion)

	if err == sql.ErrNoRows {
		respondError(c, http.StatusUnauthorized, "Invalid username or password", nil)
		return
	} else if err != nil {
		respondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	// Debug: 观测密码存储形态（前缀与长度）- 仅在非生产环境记录
	if !config.IsProduction() {
		func() {
			if storedPassword == "" {
				log.Printf("[AUTH] user=%s no password stored", req.Username)
			} else {
				prefix := storedPassword
				if len(prefix) > 7 {
					prefix = prefix[:7]
				}
				log.Printf("[AUTH] user=%s stored pw len=%d prefix=%s", req.Username, len(storedPassword), prefix)
			}
		}()
	}

	if bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(req.Password)) != nil {
		respondError(c, http.StatusUnauthorized, "Invalid username or password", nil)
		return
	}

	tokenString, err := issueClientToken(userID, req.Username, tokenVersion)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Could not generate token", err)
		return
	}

	// 同时通过 HttpOnly Cookie 下发 token，供 SSE/EventSource 等无法设置 Header 的场景使用
	// Secure 根据实际请求协议判断，避免 HTTP 访问时浏览器不发送 Cookie
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", tokenString, 24*3600, "/", "", isSecureConnection(c), true)

	c.JSON(http.StatusOK, gin.H{"token": tokenString})
}

// logout 处理 JWT 无状态注销，前端移除 token 并清除 Cookie
func logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", "", -1, "/", "", isSecureConnection(c), true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func changePassword(c *gin.Context) {
	var req UpdatePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return
	}
	if err := validateNewPassword(req.NewPassword); err != nil {
		respondError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	username := c.GetString("username")
	userID := c.GetInt("user_id")
	db := database.GetDB()

	var currentPassword string
	var tokenVersion int
	err := db.QueryRowContext(c.Request.Context(),
		"SELECT password, token_version FROM users WHERE id = ? AND username = ? AND token_version = ?",
		userID, username, c.GetInt("token_version"),
	).Scan(&currentPassword, &tokenVersion)
	if err == sql.ErrNoRows {
		respondError(c, http.StatusConflict, "Account changed concurrently, please sign in again", nil)
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	// 验证旧密码
	if bcrypt.CompareHashAndPassword([]byte(currentPassword), []byte(req.OldPassword)) != nil {
		respondError(c, http.StatusBadRequest, "Old password incorrect", nil)
		return
	}

	// 哈希新密码并更新
	newHash, herr := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if herr != nil {
		respondError(c, http.StatusInternalServerError, "Failed to hash new password", herr)
		return
	}
	result, err := db.ExecContext(c.Request.Context(),
		"UPDATE users SET password = ?, token_version = token_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ? AND password = ? AND token_version = ?",
		string(newHash), userID, username, currentPassword, tokenVersion,
	)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Failed to update password", err)
		return
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		respondError(c, http.StatusConflict, "Password changed concurrently, please sign in again", affectedErr)
		return
	}

	tokenString, err := issueClientToken(userID, username, tokenVersion+1)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Could not generate token", err)
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", tokenString, 24*3600, "/", "", isSecureConnection(c), true)

	c.JSON(http.StatusOK, gin.H{"message": "Password updated successfully", "token": tokenString})
}

func changeUsername(c *gin.Context) {
	var req UpdateUsernameRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request", err)
		return
	}
	// encoding/json replaces invalid UTF-8, so validate the original bytes too.
	body, _ := c.Get(gin.BodyBytesKey)
	if !utf8.Valid(body.([]byte)) {
		respondError(c, http.StatusBadRequest, "Username must contain valid UTF-8", nil)
		return
	}
	newUsername := strings.TrimSpace(req.NewUsername)
	if length := utf8.RuneCountInString(newUsername); length == 0 || length > 64 {
		respondError(c, http.StatusBadRequest, "Username must contain 1 to 64 characters", nil)
		return
	}
	for _, character := range newUsername {
		if unicode.IsSpace(character) || unicode.IsControl(character) {
			respondError(c, http.StatusBadRequest, "Username must not contain whitespace or control characters", nil)
			return
		}
	}

	username := c.GetString("username")
	if newUsername == username {
		respondError(c, http.StatusBadRequest, "New username must be different from the current username", nil)
		return
	}
	db := database.GetDB()
	var userID, tokenVersion int
	var currentPassword string
	err := db.QueryRowContext(c.Request.Context(),
		"SELECT id, password, token_version FROM users WHERE id = ? AND username = ? AND token_version = ?",
		c.GetInt("user_id"), username, c.GetInt("token_version"),
	).Scan(&userID, &currentPassword, &tokenVersion)
	if err == sql.ErrNoRows {
		respondError(c, http.StatusConflict, "Account changed concurrently, please sign in again", nil)
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(currentPassword), []byte(req.CurrentPassword)) != nil {
		respondError(c, http.StatusBadRequest, "Current password incorrect", nil)
		return
	}

	result, err := db.ExecContext(c.Request.Context(),
		"UPDATE users SET username = ?, token_version = token_version + 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ? AND password = ? AND token_version = ?",
		newUsername, userID, username, currentPassword, tokenVersion,
	)
	if err != nil {
		var sqliteError sqlite3.Error
		if errors.As(err, &sqliteError) && sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique {
			respondError(c, http.StatusConflict, "Username is already in use", nil)
			return
		}
		respondError(c, http.StatusInternalServerError, "Failed to update username", err)
		return
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		respondError(c, http.StatusConflict, "Account changed concurrently, please sign in again", affectedErr)
		return
	}

	tokenString, err := issueClientToken(userID, newUsername, tokenVersion+1)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "Could not generate token", err)
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("token", tokenString, 24*3600, "/", "", isSecureConnection(c), true)
	c.JSON(http.StatusOK, gin.H{"message": "Username updated successfully", "username": newUsername, "token": tokenString})
}

func issueClientToken(userID int, username string, tokenVersion int) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":       userID,
		"username":      username,
		"token_version": tokenVersion,
		"exp":           time.Now().Add(24 * time.Hour).Unix(),
	})
	return token.SignedString(jwtSecret)
}

func positiveTokenInteger(claim any) (int, bool) {
	number, ok := claim.(json.Number)
	if !ok {
		return 0, false
	}
	value, err := number.Int64()
	if err != nil || value <= 0 {
		return 0, false
	}
	integer := int(value)
	return integer, int64(integer) == value
}

func getCurrentUser(c *gin.Context) {
	username := c.GetString("username")
	c.JSON(http.StatusOK, gin.H{"username": username})
}

// AuthMiddleware validates JWT token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		// 优先从 Cookie 获取 Token（SSE/EventSource 会自动携带 Cookie）
		if tokenString == "" {
			if cookieToken, err := c.Cookie("token"); err == nil {
				tokenString = cookieToken
			}
		}
		if tokenString == "" {
			respondError(c, http.StatusUnauthorized, "Authorization header required", nil)
			c.Abort()
			return
		}

		// Remove "Bearer " prefix if present
		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		}, jwt.WithJSONNumber())

		if err != nil || !token.Valid {
			respondError(c, http.StatusUnauthorized, "Invalid token", nil)
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			respondError(c, http.StatusUnauthorized, "Invalid token claims", nil)
			c.Abort()
			return
		}

		username, ok := claims["username"].(string)
		if !ok {
			respondError(c, http.StatusUnauthorized, "Invalid token payload", nil)
			c.Abort()
			return
		}
		userID, ok := positiveTokenInteger(claims["user_id"])
		if !ok {
			respondError(c, http.StatusUnauthorized, "Session expired, please sign in again", nil)
			c.Abort()
			return
		}
		tokenVersion, ok := positiveTokenInteger(claims["token_version"])
		if !ok {
			respondError(c, http.StatusUnauthorized, "Session expired, please sign in again", nil)
			c.Abort()
			return
		}
		var currentTokenVersion int
		if err := database.GetDB().QueryRowContext(c.Request.Context(), "SELECT token_version FROM users WHERE id = ? AND username = ?", userID, username).Scan(&currentTokenVersion); err != nil || currentTokenVersion != tokenVersion {
			respondError(c, http.StatusUnauthorized, "Session expired, please sign in again", nil)
			c.Abort()
			return
		}

		c.Set("username", username)
		c.Set("user_id", userID)
		c.Set("token_version", currentTokenVersion)
		c.Next()
	}
}
