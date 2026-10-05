<template>
  <div class="login-page" role="main" aria-label="用户登录">
    <!-- 背景装饰 -->
    <div class="login-bg">
      <div class="bg-gradient"></div>
      <div class="bg-pattern"></div>
    </div>
    
    <!-- 主题切换 -->
    <button class="theme-toggle" @click="toggleDarkMode" :title="isDark ? '切换到亮色模式' : '切换到暗色模式'">
      <svg v-if="isDark" xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/></svg>
    </button>
    
    <!-- 登录卡片 -->
    <div class="login-container">
      <div ref="loginCard" class="login-card">
        <!-- Logo -->
        <div class="login-logo">
          <div class="logo-icon">
            <svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>
          </div>
          <h1 class="logo-title">TRADIS</h1>
          <p class="logo-subtitle">Docker 管理面板</p>
        </div>
        
        <!-- 普通登录表单 -->
        <form v-if="!mustChangePassword" class="login-form" @submit.prevent="handleLogin">
          <div class="form-group">
            <label class="form-label">用户名</label>
            <div class="input-wrapper">
              <span class="input-icon">
                <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
              </span>
              <input
                v-model="form.username"
                type="text"
                class="form-input"
                :class="{ 'error': errors.username }"
                placeholder="请输入用户名"
                autocomplete="username"
                @keyup.enter="handleLogin"
              />
            </div>
            <p v-if="errors.username" class="error-text">{{ errors.username }}</p>
          </div>
          
          <div class="form-group">
            <label class="form-label">密码</label>
            <div class="input-wrapper">
              <span class="input-icon">
                <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
              </span>
              <input
                v-model="form.password"
                :type="showPassword ? 'text' : 'password'"
                class="form-input"
                :class="{ 'error': errors.password }"
                placeholder="请输入密码"
                autocomplete="current-password"
                @keyup.enter="handleLogin"
              />
              <button type="button" class="toggle-password" @click="showPassword = !showPassword">
                <svg v-if="showPassword" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                <svg v-else xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7c.78 0 1.53-.09 2.24-.26"/><path d="M2 2l20 20"/></svg>
              </button>
            </div>
            <p v-if="errors.password" class="error-text">{{ errors.password }}</p>
          </div>
          
          <div class="form-options">
            <label class="remember-me">
              <input type="checkbox" v-model="form.rememberMe" />
              <span class="checkmark"></span>
              <span class="label-text">记住我</span>
            </label>
          </div>
          
          <button type="submit" class="login-btn" :disabled="loading">
            <span v-if="loading" class="btn-loader">
              <svg class="animate-spin" xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>
            </span>
            <span v-else>登 录</span>
          </button>
          
          <p v-if="errorMessage" class="form-error">{{ errorMessage }}</p>
        </form>
        
        <!-- 强制修改密码表单 -->
        <form v-else class="login-form" @submit.prevent="handleChangePassword">
          <div class="password-change-notice">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="notice-icon"><circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/></svg>
            <span>首次登录，请修改密码</span>
          </div>
          
          <div class="form-group">
            <label class="form-label">当前密码</label>
            <div class="input-wrapper">
              <input
                v-model="passwordForm.currentPassword"
                :type="showCurrentPassword ? 'text' : 'password'"
                class="form-input"
                :class="{ 'error': passwordErrors.currentPassword }"
                placeholder="请输入当前密码"
              />
              <button type="button" class="toggle-password" @click="showCurrentPassword = !showCurrentPassword">
                <svg v-if="showCurrentPassword" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                <svg v-else xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7c.78 0 1.53-.09 2.24-.26"/><path d="M2 2l20 20"/></svg>
              </button>
            </div>
            <p v-if="passwordErrors.currentPassword" class="error-text">{{ passwordErrors.currentPassword }}</p>
          </div>
          
          <div class="form-group">
            <label class="form-label">新密码</label>
            <div class="input-wrapper">
              <input
                v-model="passwordForm.newPassword"
                :type="showNewPassword ? 'text' : 'password'"
                class="form-input"
                :class="{ 'error': passwordErrors.newPassword }"
                placeholder="请输入新密码"
              />
              <button type="button" class="toggle-password" @click="showNewPassword = !showNewPassword">
                <svg v-if="showNewPassword" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                <svg v-else xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7c.78 0 1.53-.09 2.24-.26"/><path d="M2 2l20 20"/></svg>
              </button>
            </div>
            <p v-if="passwordErrors.newPassword" class="error-text">{{ passwordErrors.newPassword }}</p>
          </div>
          
          <div class="form-group">
            <label class="form-label">确认新密码</label>
            <div class="input-wrapper">
              <input
                v-model="passwordForm.confirmPassword"
                :type="showConfirmPassword ? 'text' : 'password'"
                class="form-input"
                :class="{ 'error': passwordErrors.confirmPassword }"
                placeholder="请再次输入新密码"
              />
              <button type="button" class="toggle-password" @click="showConfirmPassword = !showConfirmPassword">
                <svg v-if="showConfirmPassword" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>
                <svg v-else xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.88 9.88a3 3 0 1 0 4.24 4.24"/><path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68"/><path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7c.78 0 1.53-.09 2.24-.26"/><path d="M2 2l20 20"/></svg>
              </button>
            </div>
            <p v-if="passwordErrors.confirmPassword" class="error-text">{{ passwordErrors.confirmPassword }}</p>
          </div>
          
          <button type="submit" class="login-btn" :disabled="loading">
            <span v-if="loading" class="btn-loader">
              <svg class="animate-spin" xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>
            </span>
            <span v-else>修改密码并登录</span>
          </button>
          
          <p v-if="errorMessage" class="form-error">{{ errorMessage }}</p>
        </form>
      </div>
    </div>
    
    <!-- 版本信息 -->
    <div class="version-info">
      <span>TRADIS v0.9.7</span><!-- x-release-please-version -->
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useThemeStore } from '@/stores/theme.js'
import { api } from '@/utils/request.js'

const router = useRouter()
const route = useRoute()
const themeStore = useThemeStore()

const { isDark, toggleDarkMode } = themeStore

// 表单数据
const form = reactive({
  username: '',
  password: '',
  rememberMe: false
})

const passwordForm = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: ''
})

// 状态
const loading = ref(false)
const loginCard = ref(null)
const errorMessage = ref('')
const mustChangePassword = ref(false)
const showPassword = ref(false)
const showCurrentPassword = ref(false)
const showNewPassword = ref(false)
const showConfirmPassword = ref(false)

// 错误信息
const errors = reactive({
  username: '',
  password: ''
})

const passwordErrors = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: ''
})

// 初始化主题和记住的用户名
onMounted(() => {
  themeStore.init()

  // 抖动结束后移除类，保持 DOM 干净（下次失败由重播语义重新添加）
  loginCard.value?.addEventListener('animationend', (event) => {
    if (event.animationName === 'motion-shake') {
      loginCard.value?.classList.remove('motion-shake')
    }
  })

  // 读取记住的用户名
  const rememberedUsername = localStorage.getItem('rememberedUsername')
  if (rememberedUsername) {
    form.username = rememberedUsername
    form.rememberMe = true
  }
})

// 验证登录表单
function validateLoginForm() {
  errors.username = ''
  errors.password = ''
  
  if (!form.username.trim()) {
    errors.username = '请输入用户名'
    return false
  }
  
  if (!form.password) {
    errors.password = '请输入密码'
    return false
  }
  
  return true
}

// 验证密码修改表单
function validatePasswordForm() {
  passwordErrors.currentPassword = ''
  passwordErrors.newPassword = ''
  passwordErrors.confirmPassword = ''
  
  if (!passwordForm.currentPassword) {
    passwordErrors.currentPassword = '请输入当前密码'
    return false
  }
  
  if (!passwordForm.newPassword) {
    passwordErrors.newPassword = '请输入新密码'
    return false
  }
  
  if (passwordForm.newPassword.length < 8) {
    passwordErrors.newPassword = '新密码至少需要8个字符'
    return false
  }
  
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    passwordErrors.confirmPassword = '两次输入的密码不一致'
    return false
  }
  
  return true
}

// 显示错误：文本提示 + 卡片抖动
// 重播语义：移除类 → 强制 reflow → 重新添加，确保连续失败时抖动重新播放
function showErrorMessage(message) {
  errorMessage.value = message
  const el = loginCard.value
  if (el) {
    el.classList.remove('motion-shake')
    void el.offsetWidth
    el.classList.add('motion-shake')
  }
}

// 处理登录
async function handleLogin() {
  if (!validateLoginForm()) return
  
  loading.value = true
  errorMessage.value = ''
  
  try {
    const data = await api.auth.login({
      username: form.username,
      password: form.password
    })
    
    if (!data.error) {
      // 存储 token
      if (data.token) {
        localStorage.setItem('token', data.token)
      }
      
      // 处理记住用户名
      if (form.rememberMe) {
        localStorage.setItem('rememberedUsername', form.username)
      } else {
        localStorage.removeItem('rememberedUsername')
      }
      
      if (data.mustChangePassword) {
        mustChangePassword.value = true
        passwordForm.currentPassword = form.password
      } else {
        localStorage.setItem('loggedIn', 'true')
        const redirect = route.query.redirect || '/'
        router.push(redirect)
      }
    } else {
      showErrorMessage(data.error || '登录失败')
    }
  } catch (error) {
    showErrorMessage(error.message || '网络错误，请稍后重试')
  } finally {
    loading.value = false
  }
}

// 处理修改密码
async function handleChangePassword() {
  if (!validatePasswordForm()) return
  
  loading.value = true
  errorMessage.value = ''
  
  try {
    const data = await api.auth.changePassword({
      oldPassword: passwordForm.currentPassword,
      newPassword: passwordForm.newPassword
    })
    
    if (!data.error) {
      // 存储 token（修改密码后也会返回新 token）
      if (data.token) {
        localStorage.setItem('token', data.token)
      }
      localStorage.setItem('loggedIn', 'true')
      const redirect = route.query.redirect || '/'
      router.push(redirect)
    } else {
      showErrorMessage(data.error || '修改密码失败')
    }
  } catch (error) {
    showErrorMessage(error.message || '网络错误，请稍后重试')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
/* 使用标准 CSS，利用 CSS 变量 */
.login-page {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
  background-color: var(--bg-secondary);
}

/* 背景装饰 */
.login-bg {
  position: fixed;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}

.bg-gradient {
  position: absolute;
  inset: 0;
  background: radial-gradient(ellipse at 50% 0%, var(--primary-100) 0%, transparent 50%);
}

html.dark .bg-gradient {
  background: radial-gradient(ellipse at 50% 0%, var(--color-primary-500-15) 0%, transparent 50%);
}

.bg-pattern {
  position: absolute;
  inset: 0;
  opacity: 0.3;
  background-image: 
    radial-gradient(circle at 20% 80%, var(--primary-200) 0%, transparent 30%),
    radial-gradient(circle at 80% 20%, var(--primary-200) 0%, transparent 30%);
}

html.dark .bg-pattern {
  background-image: 
    radial-gradient(circle at 20% 80%, var(--ring-primary) 0%, transparent 30%),
    radial-gradient(circle at 80% 20%, var(--ring-primary) 0%, transparent 30%);
}

/* 主题切换按钮 */
.theme-toggle {
  position: fixed;
  top: 1rem;
  right: 1rem;
  z-index: 10;
  padding: 0.625rem;
  border-radius: 0.5rem;
  color: var(--text-secondary);
  background-color: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  box-shadow: var(--shadow-sm);
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  cursor: pointer;
}

.theme-toggle:hover {
  color: var(--text-primary);
  box-shadow: var(--shadow-md);
}

/* 登录容器 */
.login-container {
  position: relative;
  z-index: 0;
  width: 100%;
  max-width: 28rem;
}

/* 登录卡片 */
.login-card {
  background-color: var(--bg-elevated);
  border-radius: 1rem;
  box-shadow: var(--shadow-xl);
  border: 1px solid var(--border-subtle);
  padding: 2rem;
  animation: slideUp 0.5s ease-out;
}

/* scoped 规则（带 data-v 属性选择器）比特级高于全局 .motion-shake，
   必须显式覆盖 animation 才能让错误抖动生效；keyframes 与 token 引用全局定义 */
.login-card.motion-shake {
  animation: motion-shake var(--motion-duration-fast) var(--motion-ease-in-out);
}

@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(30px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  /* 抑制进入动画；motion-shake 经 token 降级（时长 1ms、位移 0px）自动失效，
     错误提示由文本消息承担 */
  .login-card {
    animation: none;
  }
}

/* Logo */
.login-logo {
  text-align: center;
  margin-bottom: 2rem;
}

.logo-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 4rem;
  height: 4rem;
  border-radius: 1rem;
  background-color: var(--color-primary-500);
  color: var(--text-inverse);
  box-shadow: 0 10px 15px -3px var(--color-primary-500-30);
  margin-bottom: 1rem;
}

.logo-title {
  font-size: 1.5rem;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.025em;
}

.logo-subtitle {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin-top: 0.25rem;
}

/* 表单 */
.login-form {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.form-group {
  position: relative;
}

.form-label {
  display: block;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-secondary);
  margin-bottom: 0.375rem;
}

.input-wrapper {
  position: relative;
  display: flex;
  align-items: center;
}

.input-icon {
  position: absolute;
  left: 0.75rem;
  color: var(--text-tertiary);
  pointer-events: none;
}

.form-input {
  width: 100%;
  padding: 0.625rem 2.5rem 0.625rem 2.5rem;
  background-color: var(--bg-primary);
  border: 1px solid var(--border-default);
  border-radius: 0.5rem;
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.form-input::placeholder {
  color: var(--text-disabled);
}

.form-input:focus {
  outline: none;
  border-color: transparent;
  box-shadow: 0 0 0 2px var(--color-primary-500);
}

.form-input.error {
  border-color: var(--color-danger-500);
}

.form-input.error:focus {
  box-shadow: 0 0 0 2px var(--color-danger-500);
}

.toggle-password {
  position: absolute;
  right: 0.75rem;
  padding: 0.25rem;
  border-radius: 0.25rem;
  color: var(--text-tertiary);
  background: none;
  border: none;
  cursor: pointer;
  transition: color var(--motion-duration-quick) var(--motion-ease-out);
}

.toggle-password:hover {
  color: var(--text-secondary);
}

.error-text {
  margin-top: 0.375rem;
  font-size: 0.875rem;
  color: var(--color-danger-600);
}

/* 选项 */
.form-options {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.remember-me {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}

.remember-me input {
  display: none;
}

.checkmark {
  width: 1rem;
  height: 1rem;
  border-radius: 0.25rem;
  border: 1px solid var(--border-default);
  background-color: var(--bg-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.remember-me input:checked + .checkmark {
  background-color: var(--color-primary-500);
  border-color: var(--color-primary-500);
}

.remember-me input:checked + .checkmark::after {
  content: '';
  width: 0.375rem;
  height: 0.625rem;
  border-right: 2px solid white;
  border-bottom: 2px solid white;
  transform: rotate(45deg) translate(-1px, -1px);
}

.label-text {
  font-size: 0.875rem;
  color: var(--text-secondary);
}

/* 登录按钮 */
.login-btn {
  width: 100%;
  padding: 0.625rem 1rem;
  border-radius: 0.5rem;
  background-color: var(--color-primary-600);
  color: var(--text-inverse);
  font-weight: 500;
  font-size: 0.875rem;
  border: none;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
  box-shadow: 0 10px 15px -3px var(--color-primary-500-30);
}

.login-btn:hover:not(:disabled) {
  background-color: var(--color-primary-700);
  box-shadow: 0 20px 25px -5px var(--color-primary-500-30);
}

.login-btn:active:not(:disabled) {
  background-color: var(--color-primary-800);
  transform: translateY(1px);
}

.login-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-loader {
  display: inline-flex;
}

.animate-spin {
  animation: spin 1s linear infinite;
}

/* 错误提示 */
.form-error {
  padding: 0.75rem;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  text-align: center;
  background-color: var(--color-danger-50);
  color: var(--color-danger-700);
  border: 1px solid var(--color-danger-200);
}

html.dark .form-error {
  background-color: rgba(239, 68, 68, 0.1);
  color: var(--color-danger-300);
  border-color: rgba(239, 68, 68, 0.3);
}

/* 密码修改提示 */
.password-change-notice {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem;
  border-radius: 0.5rem;
  margin-bottom: 1rem;
  background-color: var(--color-warning-50);
  color: var(--color-warning-700);
  border: 1px solid var(--color-warning-200);
}

html.dark .password-change-notice {
  background-color: rgba(245, 158, 11, 0.1);
  color: var(--color-warning-300);
  border-color: rgba(245, 158, 11, 0.3);
}

.notice-icon {
  flex-shrink: 0;
}

/* 版本信息 */
.version-info {
  position: fixed;
  bottom: 1rem;
  left: 50%;
  transform: translateX(-50%);
  font-size: 0.75rem;
  color: var(--text-disabled);
}
</style>
