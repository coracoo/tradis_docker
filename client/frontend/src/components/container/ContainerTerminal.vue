<template>
  <div class="terminal-container ops-console">
    <!-- 左侧会话列表 -->
    <div class="session-panel">
      <div class="panel-header">
        <h4>会话</h4>
        <button class="btn-icon" title="新建会话" aria-label="新建会话" @click="addSession({ connect: false })">
          <DynamicIcon name="plus" :size="15" />
        </button>
      </div>
      
      <ul class="session-list">
        <li 
          v-for="(session, index) in sessions" 
          :key="session.id"
          class="session-item"
          :class="{ active: activeSessionId === session.id, connected: session.status === 'connected' }"
          @click="switchSession(session.id)"
        >
          <span class="status-dot" :class="session.status" />
          <span class="session-summary">
            <span class="session-name">{{ session.cmd }}</span>
            <small>{{ getSessionUserLabel(session) }}</small>
          </span>
          <button class="btn-close" title="关闭会话" aria-label="关闭会话" @click.stop="removeSession(index)">
            <DynamicIcon name="x" :size="13" />
          </button>
        </li>
      </ul>
      
      <div class="panel-actions">
        <button class="action-link" @click="copySelection">
          <DynamicIcon name="copy" :size="13" />
          复制
        </button>
        <button class="action-link" @click="pasteClipboard">
          <DynamicIcon name="paste" :size="13" />
          粘贴
        </button>
      </div>
    </div>
    
    <!-- 右侧终端区域 -->
    <div class="terminal-panel">
      <!-- 每个会话有自己的终端容器 -->
      <div
        v-for="session in sessions"
        :key="session.id"
        :id="'terminal-' + session.id"
        class="terminal-instance"
        :class="{ active: activeSessionId === session.id }"
      />
      <div v-if="activeSession && activeSession.status !== 'connected'" class="connection-state">
        <div class="connection-form">
          <div class="connection-heading">
            <span class="connection-icon"><DynamicIcon name="terminal" :size="18" /></span>
            <span>
              <strong>{{ getConnectionText(activeSession) }}</strong>
              <small>选择容器内可用的 Shell 和执行用户</small>
            </span>
          </div>

          <div class="connection-fields">
            <label class="connection-field">
              <span>Shell</span>
              <select v-model="activeSession.cmd" :disabled="activeSession.status === 'connecting'">
                <option value="/bin/sh">/bin/sh</option>
                <option value="/bin/bash">/bin/bash</option>
                <option value="/bin/ash">/bin/ash</option>
              </select>
            </label>
            <label class="connection-field">
              <span>执行用户</span>
              <select v-model="activeSession.userMode" :disabled="activeSession.status === 'connecting'">
                <option value="default">容器默认用户</option>
                <option value="root">root</option>
                <option value="nobody">nobody</option>
                <option value="custom">指定用户</option>
              </select>
            </label>
          </div>

          <label v-if="activeSession.userMode === 'custom'" class="connection-field custom-user-field">
            <span>用户名或 UID</span>
            <input
              v-model.trim="activeSession.customUser"
              :disabled="activeSession.status === 'connecting'"
              autocomplete="off"
              placeholder="例如 app、1000 或 1000:1000"
            />
          </label>

          <p v-if="activeSession.errorMessage" class="connection-error" role="alert">
            <DynamicIcon name="circle-alert" :size="14" />
            {{ activeSession.errorMessage }}
          </p>
          <p v-else class="connection-hint">
            “容器默认用户”沿用镜像的 USER 配置；root 仅在排障或维护时使用。
          </p>

          <button
            class="btn-connect"
            :disabled="activeSession.status === 'connecting'"
            @click="connectSession(activeSession)"
          >
            <DynamicIcon :name="activeSession.status === 'connecting' ? 'loading' : 'terminal'" :size="14" />
            {{ activeSession.status === 'connecting' ? '正在准备终端...' : activeSession.status === 'idle' ? '连接终端' : '重新连接' }}
          </button>
        </div>
      </div>
      <div v-if="sessions.length === 0" class="terminal-placeholder">
        点击左侧“新建会话”配置终端连接
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { useUiStore } from '@/stores/ui.js'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import '@xterm/xterm/css/xterm.css'

const uiStore = useUiStore()

const props = defineProps({
  containerId: { type: String, required: true }
})

const sessions = ref([])
const activeSessionId = ref('')
let themeObserver = null

const activeSession = computed(() => sessions.value.find(session => session.id === activeSessionId.value))
const terminalUserPattern = /^[A-Za-z0-9_.-]+(?::[A-Za-z0-9_.-]+)?$/

const resolveSessionUser = (session) => {
  if (session.userMode === 'root') return 'root'
  if (session.userMode === 'nobody') return 'nobody'
  if (session.userMode === 'custom') return String(session.customUser || '').trim()
  return ''
}

const getSessionUserLabel = (session) => resolveSessionUser(session) || '默认用户'

// 检测深色模式
const isDarkMode = () => document.documentElement.classList.contains('dark')

// 获取 CSS 变量值
const getCssVariable = (name) => {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim()
}

// 获取终端主题配置
const getTerminalTheme = () => {
  const bgPrimary = getCssVariable('--bg-primary')
  const bgSecondary = getCssVariable('--bg-secondary')
  const bgTertiary = getCssVariable('--bg-tertiary')
  const textPrimary = getCssVariable('--text-primary')
  const textSecondary = getCssVariable('--text-secondary')
  const textTertiary = getCssVariable('--text-tertiary')
  const primaryColor = getCssVariable('--color-primary-500')
  const primaryColor600 = getCssVariable('--color-primary-600')
  const dangerColor = getCssVariable('--color-danger-500')
  const dangerColor600 = getCssVariable('--color-danger-600')
  const successColor = getCssVariable('--color-success-500')
  const successColor600 = getCssVariable('--color-success-600')
  const warningColor = getCssVariable('--color-warning-500')
  const warningColor600 = getCssVariable('--color-warning-600')
  const gray400 = getCssVariable('--color-gray-400')
  const gray500 = getCssVariable('--color-gray-500')
  const gray600 = getCssVariable('--color-gray-600')
  const gray700 = getCssVariable('--color-gray-700')
  const gray800 = getCssVariable('--color-gray-800')
  const gray900 = getCssVariable('--color-gray-900')
  
  if (isDarkMode()) {
    return {
      background: bgSecondary,
      foreground: textPrimary,
      cursor: textSecondary,
      selectionBackground: getCssVariable('--color-primary-500-20'),
      selectionForeground: textPrimary,
      selectionInactiveBackground: bgTertiary,
      cursorAccent: bgSecondary,
      black: gray900,
      red: dangerColor,
      green: successColor,
      yellow: warningColor,
      blue: primaryColor,
      magenta: getCssVariable('--color-primary-600'),
      cyan: getCssVariable('--color-info-500'),
      white: textSecondary,
      brightBlack: gray600,
      brightRed: getCssVariable('--color-danger-400'),
      brightGreen: getCssVariable('--color-success-400'),
      brightYellow: getCssVariable('--color-warning-400'),
      brightBlue: getCssVariable('--color-primary-400'),
      brightMagenta: getCssVariable('--color-primary-300'),
      brightCyan: getCssVariable('--color-info-400'),
      brightWhite: textPrimary
    }
  }
  return {
    background: bgPrimary,
    foreground: gray900,
    cursor: gray700,
    selectionBackground: getCssVariable('--color-primary-100'),
    selectionForeground: gray900,
    selectionInactiveBackground: bgSecondary,
    cursorAccent: bgPrimary,
    black: gray900,
    red: dangerColor600,
    green: successColor600,
    yellow: warningColor600,
    blue: primaryColor600,
    magenta: getCssVariable('--color-primary-700'),
    cyan: getCssVariable('--color-info-600'),
    white: getCssVariable('--color-gray-100'),
    brightBlack: gray600,
    brightRed: dangerColor,
    brightGreen: successColor,
    brightYellow: warningColor,
    brightBlue: primaryColor,
    brightMagenta: getCssVariable('--color-primary-500'),
    brightCyan: getCssVariable('--color-info-500'),
    brightWhite: getCssVariable('--bg-elevated')
  }
}

// 为每个会话创建独立的终端
const createTerminalForSession = (session) => {
  const container = document.getElementById('terminal-' + session.id)
  if (!container || session.terminal) return

  const terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: getCssVariable('--font-mono') || '"JetBrains Mono", Consolas, monospace',
    scrollback: 1000,
    minimumContrastRatio: 4.5,
    drawBoldTextInBrightColors: true,
    theme: getTerminalTheme()
  })

  const fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)

  terminal.open(container)
  fitAddon.fit()

  // 保存到会话
  session.terminal = terminal
  session.fitAddon = fitAddon
  session.terminalHost = container

  // 输入转发
  terminal.onData(data => {
    if (session.socket?.readyState === WebSocket.OPEN) {
      session.socket.send(JSON.stringify({ type: 'input', data }))
    }
  })

  return terminal
}

const resizeSession = (session) => {
  if (!session?.terminal || !session.fitAddon) return

  session.fitAddon.fit()
  session.terminal.refresh(0, Math.max(session.terminal.rows - 1, 0))

  if (session.socket?.readyState === WebSocket.OPEN) {
    session.socket.send(JSON.stringify({
      type: 'resize',
      data: JSON.stringify({
        rows: session.terminal.rows,
        cols: session.terminal.cols
      })
    }))
  }
}

const activateSessionTerminal = async (session, focus = false) => {
  if (!session || session.disposed) return

  activeSessionId.value = session.id
  await nextTick()
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      resizeSession(session)
      if (focus) {
        session.terminal?.focus()
      }
    })
  })
}

// 连接 WebSocket
const connectSession = (session) => {
  if (!session || session.disposed || session.status === 'connecting' || session.socket?.readyState === WebSocket.OPEN) {
    return
  }

  const user = resolveSessionUser(session)
  if (session.userMode === 'custom' && !user) {
    session.errorMessage = '请输入要使用的用户名或 UID'
    return
  }
  if (user && !terminalUserPattern.test(user)) {
    session.errorMessage = '执行用户仅支持用户名、UID 或 user:group 格式'
    return
  }

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const query = new URLSearchParams({
    cmd: session.cmd
  })
  if (user) query.set('user', user)
  const wsUrl = `${protocol}//${host}/api/containers/${props.containerId}/terminal?${query.toString()}`

  try {
    session.socket?.close()
    session.status = 'connecting'
    session.errorMessage = ''
    session.requestedUser = user
    const socket = new WebSocket(wsUrl)
    session.socket = socket

    socket.onopen = () => {
      if (session.disposed || session.socket !== socket) {
        socket.close()
      }
    }

    socket.onmessage = (event) => {
      if (typeof event.data === 'string') {
        let control = null
        try {
          control = JSON.parse(event.data)
        } catch {
          session.terminal?.write(event.data)
          return
        }

        if (control.type === 'ready') {
          session.status = 'connected'
          session.errorMessage = ''
          const connectedUser = control.data?.user || getSessionUserLabel(session)
          session.terminal?.writeln(
            `\x1b[36m[TRADIS]\x1b[0m \x1b[32m已连接\x1b[0m \x1b[2m${session.cmd} · ${connectedUser}\x1b[0m`
          )
          activateSessionTerminal(session, true)
          return
        }
        if (control.type === 'error') {
          session.status = 'error'
          session.errorMessage = control.data?.message || '终端初始化失败'
          return
        }

        session.terminal?.write(event.data)
      } else {
        const reader = new FileReader()
        reader.onload = () => session.terminal?.write(reader.result)
        reader.readAsText(event.data, 'utf-8')
      }
    }

    socket.onclose = () => {
      if (session.socket !== socket) return
      session.socket = null
      if (session.disposed) return
      if (session.status !== 'error') {
        session.status = 'closed'
        session.errorMessage = '终端连接已关闭'
        session.terminal?.writeln('\r\n\x1b[33m[连接已关闭，请手动重连]\x1b[0m')
      }
    }

    socket.onerror = () => {
      if (session.socket === socket && !session.disposed) {
        session.status = 'error'
        session.errorMessage = session.errorMessage || '无法建立终端连接，请检查容器状态和连接配置'
      }
    }

  } catch (err) {
    session.status = 'error'
    session.errorMessage = `连接失败：${err.message}`
  }
}

const getConnectionText = (session) => {
  const labels = {
    idle: `已创建 ${session.cmd} 会话`,
    connecting: `正在连接 ${session.cmd}`,
    closed: `${session.cmd} 会话已断开`,
    error: `${session.cmd} 连接失败`
  }
  return labels[session.status] || '终端未连接'
}

// 添加会话
const addSession = async (options = {}) => {
  const cmd = options.cmd || '/bin/sh'
  const shouldConnect = options.connect !== false
  const session = {
    id: `s-${Date.now()}`,
    cmd,
    socket: null,
    terminal: null,
    fitAddon: null,
    status: 'idle',
    userMode: options.userMode || 'default',
    customUser: options.customUser || '',
    requestedUser: '',
    errorMessage: '',
    disposed: false
  }

  sessions.value.push(session)
  activeSessionId.value = session.id

  // 等待 DOM 更新后创建终端
  await nextTick()
  createTerminalForSession(session)
  if (shouldConnect) {
    connectSession(session)
  }
}

// 切换会话
const switchSession = (id) => {
  const session = sessions.value.find(s => s.id === id)
  if (!session) return

  // 如果终端未创建，创建它
  if (!session.terminal) {
    nextTick(() => createTerminalForSession(session))
  }

  activateSessionTerminal(session, session.status === 'connected')
}

// 移除会话
const removeSession = (index) => {
  const session = sessions.value[index]
  if (!session) return

  session.disposed = true
  // 关闭连接
  session.socket?.close()
  // 销毁终端
  session.terminal?.dispose()

  sessions.value.splice(index, 1)

  // 如果删除的是当前会话，切换到其他会话
  if (activeSessionId.value === session.id) {
    activeSessionId.value = sessions.value[0]?.id || ''
  }
}

// 复制
const copySelection = async () => {
  const session = sessions.value.find(s => s.id === activeSessionId.value)
  const text = session?.terminal?.getSelection()
  if (!text) return

  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const textarea = document.createElement('textarea')
    textarea.value = text
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
  }
}

// 粘贴
const pasteClipboard = async () => {
  const session = sessions.value.find(s => s.id === activeSessionId.value)
  if (!session?.socket || session.socket.readyState !== WebSocket.OPEN) {
    uiStore.toastWarning('当前会话未连接')
    return
  }

  try {
    const text = await navigator.clipboard.readText()
    session.socket.send(JSON.stringify({ type: 'input', data: text }))
  } catch {
    uiStore.toastError('无法读取剪贴板')
  }
}

// 窗口大小变化
const handleResize = () => {
  const session = sessions.value.find(s => s.id === activeSessionId.value)
  if (!session?.terminal) return

  resizeSession(session)
}

// 监听主题变化并更新终端
const handleThemeChange = () => {
  const newTheme = getTerminalTheme()
  sessions.value.forEach(session => {
    if (session.terminal) {
      session.terminal.options.theme = newTheme
    }
  })
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  // 监听主题变化
  themeObserver = new MutationObserver((mutations) => {
    mutations.forEach((mutation) => {
      if (mutation.attributeName === 'class') {
        handleThemeChange()
      }
    })
  })
  themeObserver.observe(document.documentElement, { attributes: true })
  addSession({ cmd: '/bin/sh', connect: false })
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  themeObserver?.disconnect()
  // 清理所有会话
  sessions.value.forEach(s => {
    s.disposed = true
    s.socket?.close()
    s.terminal?.dispose()
  })
})

// 监听容器ID变化
watch(() => props.containerId, async () => {
  // 关闭所有现有会话
  sessions.value.forEach(s => {
    s.disposed = true
    s.socket?.close()
    s.terminal?.dispose()
  })
  sessions.value = []
  activeSessionId.value = ''
  await addSession({ cmd: '/bin/sh', connect: false })
})
</script>

<style scoped>
.terminal-container {
  display: flex;
  height: 500px;
  background: var(--bg-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  overflow: hidden;
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
}

/* 左侧会话面板 */
.session-panel {
  width: 212px;
  background: color-mix(in srgb, var(--bg-secondary) 76%, var(--bg-elevated));
  border-right: 1px solid var(--border-subtle);
  display: flex;
  flex-direction: column;
  flex-shrink: 0;
}

.panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  min-height: 52px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--border-subtle);
}

.panel-header h4 {
  margin: 0;
  font-size: 0.875rem;
  color: var(--text-primary);
}

.btn-icon {
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-primary-500);
  border: none;
  border: 1px solid var(--color-primary-600);
  border-radius: 8px;
  color: var(--text-inverse);
  cursor: pointer;
}

.session-list {
  flex: 1;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 8px;
}

.session-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  padding: 7px 9px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 0.8125rem;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.session-item:hover {
  background: var(--bg-tertiary);
}

.session-item.active {
  background: var(--color-primary-100);
  color: var(--color-primary-700);
  box-shadow: inset 3px 0 0 var(--color-primary-500);
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.status-dot.connected {
  background: var(--color-success-500);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-success-500) 16%, transparent);
}

.status-dot.connecting {
  background: var(--color-warning-500);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-warning-500) 14%, transparent);
}

.status-dot.error {
  background: var(--color-danger-500);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--color-danger-500) 14%, transparent);
}

.status-dot.idle,
.status-dot.closed {
  background: var(--text-tertiary);
}

.session-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.75rem;
}

.session-summary {
  display: grid;
  flex: 1;
  min-width: 0;
  gap: 1px;
}

.session-summary small {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 0.625rem;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.btn-close {
  width: 22px;
  height: 22px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: var(--text-tertiary);
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.2s;
}

.session-item:hover .btn-close {
  opacity: 1;
}

.btn-close:hover {
  color: var(--color-danger-500);
}

.panel-actions {
  display: flex;
  gap: 12px;
  padding: 12px 16px;
  border-top: 1px solid var(--border-subtle);
}

.action-link {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  background: transparent;
  border: none;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  cursor: pointer;
  padding: 0;
}

.action-link:hover {
  color: var(--text-primary);
}

/* 右侧终端面板 */
.terminal-panel {
  flex: 1;
  position: relative;
  overflow: hidden;
  background: var(--bg-primary);
}

/* 每个会话的终端实例 */
.terminal-instance {
  position: absolute;
  inset: 0;
  opacity: 0;
  visibility: hidden;
  transition: opacity 0.1s ease;
  padding: 12px;
}

.terminal-instance.active {
  opacity: 1;
  visibility: visible;
  z-index: 1;
}

.connection-state {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: color-mix(in srgb, var(--bg-primary) 94%, var(--bg-secondary));
}

.connection-form {
  display: grid;
  width: min(100%, 520px);
  gap: 16px;
  padding: 22px;
  background: var(--bg-elevated);
  border: 1px solid var(--border-default);
  border-radius: 8px;
  box-shadow: 0 16px 40px color-mix(in srgb, var(--text-primary) 8%, transparent);
}

.connection-heading {
  display: flex;
  align-items: center;
  gap: 11px;
}

.connection-heading > span:last-child {
  display: grid;
  gap: 2px;
}

.connection-heading strong {
  color: var(--text-primary);
  font-size: 0.9375rem;
  font-weight: 650;
}

.connection-heading small {
  color: var(--text-tertiary);
  font-size: 0.75rem;
}

.connection-icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  color: var(--color-primary-600);
  background: var(--color-primary-100);
  border: 1px solid color-mix(in srgb, var(--color-primary-500) 24%, var(--border-subtle));
  border-radius: 8px;
}

.connection-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.connection-field {
  display: grid;
  gap: 6px;
}

.connection-field > span {
  color: var(--text-secondary);
  font-size: 0.6875rem;
  font-weight: 700;
}

.connection-field select,
.connection-field input {
  width: 100%;
  min-height: 36px;
  padding: 0 10px;
  color: var(--text-primary);
  background: var(--bg-primary);
  border: 1px solid var(--border-default);
  border-radius: 7px;
  outline: none;
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
  font-size: 0.75rem;
}

.connection-field select:focus,
.connection-field input:focus {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--color-primary-500-15);
}

.connection-field select:disabled,
.connection-field input:disabled {
  opacity: 0.65;
  cursor: wait;
}

.custom-user-field {
  margin-top: -4px;
}

.connection-hint {
  margin: -2px 0 0;
  color: var(--text-tertiary);
  font-size: 0.6875rem;
  line-height: 1.5;
}

.connection-error {
  display: flex;
  align-items: flex-start;
  gap: 7px;
  margin: -2px 0 0;
  color: var(--color-danger-600);
  font-size: 0.75rem;
  line-height: 1.45;
}

.btn-connect {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  justify-self: end;
  min-width: 128px;
  min-height: 36px;
  padding: 0 14px;
  border: 1px solid var(--color-primary-600);
  border-radius: 8px;
  background: var(--color-primary-500);
  color: var(--text-inverse);
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
}

.btn-connect:hover:not(:disabled) {
  background: var(--color-primary-600);
}

.btn-connect:disabled {
  opacity: 0.65;
  cursor: wait;
}

.terminal-placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-tertiary);
  font-size: 0.875rem;
}

:deep(.xterm) {
  height: 100%;
}

:deep(.xterm-screen) {
  height: 100% !important;
}

:deep(.xterm-viewport::-webkit-scrollbar) {
  width: 8px;
}

:deep(.xterm-viewport::-webkit-scrollbar-thumb) {
  background: var(--border-default);
  border-radius: 4px;
}

@media (max-width: 720px) {
  .terminal-container {
    flex-direction: column;
  }

  .session-panel {
    width: 100%;
    max-height: 168px;
    border-right: 0;
    border-bottom: 1px solid var(--border-subtle);
  }

  .session-list {
    display: flex;
    gap: 6px;
    overflow-x: auto;
  }

  .session-item {
    flex: 0 0 150px;
    margin-bottom: 0;
  }

  .connection-state {
    padding: 14px;
  }

  .connection-form {
    padding: 16px;
  }

  .connection-fields {
    grid-template-columns: 1fr;
  }
}
</style>
