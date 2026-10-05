package api

import (
	"dockerpanel/backend/pkg/database"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	return r
}

func TestLogin_Success(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "current-password")
	if err := database.InitDB(filepath.Join(t.TempDir(), "auth.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"current-password"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	login(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"token"`) {
		t.Fatalf("missing token in response: %s", recorder.Body.String())
	}
}

func TestLogin_InvalidRequest(t *testing.T) {
	r := setupTestRouter()
	RegisterAuthRoutes(r)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":""}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLoginRejectsPlaintextStoredPassword(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "current-password")
	if err := database.InitDB(filepath.Join(t.TempDir(), "auth.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if _, err := database.GetDB().Exec("UPDATE users SET password = ? WHERE username = ?", "legacy-password", "admin"); err != nil {
		t.Fatalf("seed plaintext password: %v", err)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"legacy-password"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	login(context)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var stored string
	if err := database.GetDB().QueryRow("SELECT password FROM users WHERE username = ?", "admin").Scan(&stored); err != nil {
		t.Fatalf("read password: %v", err)
	}
	if stored != "legacy-password" {
		t.Fatalf("plaintext password was unexpectedly migrated: %q", stored)
	}
}

func TestLogin_MethodNotAllowed(t *testing.T) {
	r := setupTestRouter()
	RegisterAuthRoutes(r)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/auth/login", nil)
	r.ServeHTTP(w, req)

	// POST only endpoint
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestValidateNewPassword(t *testing.T) {
	if err := validateNewPassword("short"); err == nil {
		t.Fatal("expected short password to be rejected")
	}
	if err := validateNewPassword("12345678"); err != nil {
		t.Fatalf("expected 8-character password to be accepted: %v", err)
	}
}

func TestPasswordChangeRevokesExistingToken(t *testing.T) {
	_ = database.Close()
	t.Setenv("ADMIN_PASSWORD", "current-password")
	if err := database.InitDB(filepath.Join(t.TempDir(), "auth.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	loginRecorder := httptest.NewRecorder()
	loginContext, _ := gin.CreateTestContext(loginRecorder)
	loginContext.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"current-password"}`))
	loginContext.Request.Header.Set("Content-Type", "application/json")
	login(loginContext)
	if loginRecorder.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRecorder.Code, loginRecorder.Body.String())
	}
	var loginPayload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(loginRecorder.Body.Bytes(), &loginPayload); err != nil {
		t.Fatal(err)
	}

	changeRecorder := httptest.NewRecorder()
	changeContext, _ := gin.CreateTestContext(changeRecorder)
	changeContext.Set("username", "admin")
	changeContext.Request = httptest.NewRequest(http.MethodPost, "/api/auth/change-password", strings.NewReader(`{"oldPassword":"current-password","newPassword":"new-password"}`))
	changeContext.Request.Header.Set("Content-Type", "application/json")
	changePassword(changeContext)
	if changeRecorder.Code != http.StatusOK {
		t.Fatalf("change status=%d body=%s", changeRecorder.Code, changeRecorder.Body.String())
	}

	router := setupTestRouter()
	router.GET("/protected", AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+loginPayload.Token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("old token status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLoginRateLimitMiddleware(t *testing.T) {
	loginRateMu.Lock()
	originalLimit := loginRateLimit
	loginRateLimit = 1
	loginRateByIP = map[string]loginRateState{}
	loginRateMu.Unlock()
	t.Cleanup(func() {
		loginRateMu.Lock()
		loginRateLimit = originalLimit
		loginRateByIP = map[string]loginRateState{}
		loginRateMu.Unlock()
	})

	r := setupTestRouter()
	r.POST("/login", loginRateLimitMiddleware(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/login", nil))
	if first.Code != http.StatusNoContent {
		t.Fatalf("first request status = %d", first.Code)
	}

	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/login", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d", second.Code)
	}
}

func TestJWTGeneration(t *testing.T) {
	// 测试JWT生成
	secret := []byte("test-secret-key-for-jwt-generation")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "testuser",
		"exp":      time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString(secret)
	assert.NoError(t, err)
	assert.NotEmpty(t, tokenString)

	// 验证JWT
	parsedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	assert.NoError(t, err)
	assert.True(t, parsedToken.Valid)

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	assert.True(t, ok)
	assert.Equal(t, "testuser", claims["username"])
}

func TestJWTExpired(t *testing.T) {
	secret := []byte("test-secret-key")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "testuser",
		"exp":      time.Now().Add(-time.Hour).Unix(), // 已过期的token
	})

	tokenString, _ := token.SignedString(secret)

	// 验证已过期的JWT
	_, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return secret, nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestAuthMiddleware_NoToken(t *testing.T) {
	r := setupTestRouter()
	authGroup := r.Group("/api/protected")
	authGroup.Use(AuthMiddleware())
	authGroup.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/protected/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	r := setupTestRouter()
	authGroup := r.Group("/api/protected")
	authGroup.Use(AuthMiddleware())
	authGroup.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/protected/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_InvalidTokenFormat(t *testing.T) {
	r := setupTestRouter()
	authGroup := r.Group("/api/protected")
	authGroup.Use(AuthMiddleware())
	authGroup.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/protected/test", nil)
	req.Header.Set("Authorization", "Basic invalid-token") // 错误的前缀
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthMiddleware_ValidTokenFormat(t *testing.T) {
	// 注意：这需要设置JWT_SECRET环境变量，这里仅测试格式
	// 完整的集成测试需要真实的数据库和用户

	// 创建一个有效的token（使用当前JWT_SECRET）
	secret := []byte("test-secret-key")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": "admin",
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString(secret)

	r := setupTestRouter()
	authGroup := r.Group("/api/protected")
	authGroup.Use(AuthMiddleware())
	authGroup.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/protected/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	// 注意：这里会失败，因为jwtSecret没有设置为test-secret-key
	// 这是一个示例，展示如何测试
	r.ServeHTTP(w, req)

	// 期望401，因为JWT_SECRET不匹配
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
