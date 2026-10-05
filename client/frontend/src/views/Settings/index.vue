<template>
  <ResourceWorkbench class="settings-workbench">
    <template #toolbar>
      <div class="toolbar workbench-toolbar settings-toolbar">
        <div class="toolbar-left workbench-toolbar-left">
          <SearchInput v-model="settingsSearchQuery" placeholder="搜索设置名称、功能或配置项..." />
          <SegmentedTabs
            class="settings-category-tabs"
            data-tour="settings-categories"
            :model-value="activeSettingCategory"
            :options="settingCategoryTabs"
            aria-label="设置分类"
            panel-id="settings-tab-panel"
            @update:model-value="selectSettingCategory"
          />
        </div>
        <div class="toolbar-right">
          <button class="secondary-btn" :disabled="loading" @click="refreshSettingsPage">
            <DynamicIcon name="refresh" :size="16" />
            刷新
          </button>
        </div>
      </div>
    </template>

    <div id="settings-tab-panel" ref="settingsPageRef" class="settings-page" role="tabpanel" aria-label="系统设置">
    <div v-if="visibleSettingSections.size === 0" class="settings-empty">
      <DynamicIcon name="search" :size="24" />
      <strong>没有匹配的设置</strong>
      <span>尝试搜索设置名称、功能或配置项</span>
    </div>
    <!-- 当前分类设置 -->
    <div v-else class="settings-columns">
        <!-- 外观设置 -->
        <div id="appearance-settings" v-if="isSettingSectionVisible('appearance')" class="settings-card" data-setting-key="appearance">
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="palette" :size="18" class="card-icon" />
              外观设置
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <label class="form-label">主题色</label>
              <div class="color-theme-selector">
                <button
                  v-ripple
                  v-for="theme in colorThemes" 
                  :key="theme.id"
                  class="color-theme-btn"
                  :class="{ active: currentTheme === theme.id }"
                  :style="{ '--theme-color': theme.color }"
                  @click="setTheme(theme.id)"
                >
                  <span class="color-dot" :style="{ backgroundColor: theme.color }"></span>
                  <span class="color-name">{{ theme.name }}</span>
                </button>
              </div>
              <p class="form-hint">选择喜欢的主题色，改变界面整体色调</p>
            </div>
            <div class="form-group">
              <SwitchToggle :model-value="isDark" label="暗色模式" @update:model-value="toggleDarkMode" />
              <p class="form-hint">切换亮色/暗色主题</p>
            </div>
            <div class="form-group">
              <label class="form-label">图标风格</label>
              <div class="icon-set-selector">
                <button
                  v-ripple
                  v-for="set in iconSets" 
                  :key="set.id"
                  class="icon-set-btn"
                  :class="{ active: iconStore.iconSet === set.id }"
                  @click="setIconSet(set.id)"
                >
                  <span class="icon-preview">
                    <IconLucideBox v-if="set.id === 'lucide'" class="w-5 h-5" />
                    <IconTablerBox v-else class="w-5 h-5" />
                  </span>
                  <span class="icon-set-name">{{ set.name }}</span>
                </button>
              </div>
              <p class="form-hint">选择界面图标风格（Lucide 或 Tabler）</p>
            </div>
          </div>
        </div>

        <!-- 安全设置 -->
        <div id="security-settings" v-if="isSettingSectionVisible('security')" class="settings-card" data-setting-key="security">
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="shield" :size="18" class="card-icon" />
              安全设置
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <label class="form-label">修改管理员密码</label>
              <input v-model="passwordForm.oldPassword" type="password" class="form-input" placeholder="当前密码" />
              <input v-model="passwordForm.newPassword" type="password" class="form-input mt-2" placeholder="新密码" />
              <input v-model="passwordForm.confirmPassword" type="password" class="form-input mt-2" placeholder="确认新密码" />
              <div class="form-actions">
                <button v-ripple class="btn btn-primary" :class="{ 'is-loading': passwordLoading }" :disabled="passwordLoading" @click="updatePassword">
                  <span v-if="passwordLoading" class="btn-spinner"></span>
                  更新密码
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- 新手引导 -->
        <div id="onboarding-settings" v-if="isSettingSectionVisible('onboarding')" class="settings-card" data-setting-key="onboarding">
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="compass" :size="18" class="card-icon" />
              新手引导
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <SwitchToggle
                :model-value="onboarding.showEntryBanner"
                label="显示新手引导横幅"
                @update:model-value="onToggleOnboardingBanner"
              />
              <p class="setting-hint">关闭后横幅不再出现；想再看可在此重新打开，或随时在仪表盘点「新手引导」。</p>
            </div>
          </div>
        </div>

        <!-- 高级选项 -->
        <div
          v-if="isSettingSectionVisible('advanced')"
          id="advanced-settings"
          class="settings-card"
          :class="{ 'settings-column-start': isSettingColumnStart('advanced') }"
          data-setting-key="advanced"
        >
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="settings" :size="18" class="card-icon" />
              高级选项
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <SwitchToggle
                :model-value="settingsForm.advancedMode"
                :disabled="advancedModeLoading"
                label="启用高级模式"
                @update:model-value="saveAdvancedMode"
              />
              <p class="form-hint">开启后将允许修改并保存高风险的 YAML 配置，建议仅在明确知道修改内容时使用。</p>
            </div>
          </div>
        </div>

        <!-- 系统诊断 -->
        <div
          v-if="isSettingSectionVisible('diagnostics')"
          id="diagnostics-settings"
          class="settings-card"
          data-setting-key="diagnostics"
        >
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="download" :size="18" class="card-icon" />
              系统诊断
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <p class="form-hint diagnostic-description">
                导出版本、Docker 运行信息、近期任务与系统事件，便于排查面板问题。
              </p>
              <p class="form-hint">诊断包不包含密钥、Token、项目配置或容器日志。</p>
              <div class="form-actions">
                <button
                  v-ripple
                  type="button"
                  class="btn btn-default"
                  :class="{ 'is-loading': diagnosticExporting }"
                  :disabled="diagnosticExporting"
                  @click="exportDiagnosticBundle"
                >
                  <DynamicIcon name="download" :size="16" />
                  导出诊断包
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- 服务配置 -->
        <div id="service-settings" v-if="isSettingSectionVisible('service')" class="settings-card" data-setting-key="service">
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="server" :size="18" class="card-icon" />
              服务配置
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <label class="form-label">内网服务器地址</label>
              <input v-model="settingsForm.lanUrl" class="form-input" placeholder="http://192.168.1.100" />
              <p class="form-hint">用于自动生成内网访问的容器导航链接</p>
            </div>
            <div class="form-group">
              <label class="form-label">外网服务器地址</label>
              <input v-model="settingsForm.wanUrl" class="form-input" placeholder="https://example.com" />
              <p class="form-hint">用于自动生成外网访问的容器导航链接</p>
            </div>
            <EditionSettingsFields ref="fullServiceFieldsRef" v-model:fields="editionFields" section="service" />
            <div class="form-group">
              <label class="form-label">应用商店 CDN 地址</label>
              <input v-model="settingsForm.appStoreCDNURL" class="form-input" placeholder="https://tradis-templates.coracoo.deno.net" />
              <p v-if="!isFullEdition || cdnStatus?.speedtestDisabled" class="form-hint">模板读取优先使用 CDN</p>
              <p v-else class="form-hint">模板读取优先使用 CDN；地址变化及每 24 小时会自动执行 Cloudflare 优选测速</p>
              <div v-if="isFullEdition && !cdnStatus?.speedtestDisabled" class="cdn-status" :class="`is-${cdnStatusType}`">
                <DynamicIcon :name="cdnStatus?.running ? 'refresh' : 'network'" :size="16" :class="{ 'spin-icon': cdnStatus?.running }" />
                <div class="cdn-status-content">
                  <span>{{ cdnStatusText }}</span>
                  <span v-if="cdnStatus?.lastTestTime && cdnStatus?.bestIp" class="cdn-status-time">
                    上次测速：{{ formatCDNTestTime(cdnStatus.lastTestTime) }}
                  </span>
                </div>
              </div>
            </div>
            <div class="form-group">
              <label class="form-label">镜像更新检查间隔（分钟）</label>
              <input v-model.number="settingsForm.imageUpdateIntervalMinutes" type="number" min="5" max="720" class="form-input" />
              <p class="form-hint">控制全局镜像更新检测的时间间隔，默认 120 分钟</p>
            </div>
            <div class="form-actions">
              <button v-ripple class="btn btn-primary" :class="{ 'is-loading': serverLoading }" :disabled="serverLoading" @click="saveServerSettings">
                <span v-if="serverLoading" class="btn-spinner"></span>
                保存配置
              </button>
              <button
                v-if="isFullEdition && !cdnStatus?.speedtestDisabled"
                v-ripple
                type="button"
                class="btn btn-default"
                :class="{ 'is-loading': cdnRefreshing }"
                :disabled="cdnRefreshing || !settingsForm.appStoreCDNURL.trim()"
                @click="refreshCDNBestIP"
              >
                <span v-if="cdnRefreshing" class="btn-spinner"></span>
                刷新优选 IP
              </button>
            </div>
          </div>
        </div>

        <!-- NAS 部署默认值 -->
        <div
          v-if="isSettingSectionVisible('nasDefaults')"
          id="nas-defaults-settings"
          class="settings-card"
          data-setting-key="nasDefaults"
        >
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="server-cog" :size="18" class="card-icon" />
              NAS 部署默认值
            </h3>
          </div>
          <div class="card-body">
            <div class="profile-identity-grid">
              <div class="form-group">
                <label class="form-label">默认 PUID</label>
                <input v-model.number="deploymentPolicyForm.puid" type="number" min="0" class="form-input" />
              </div>
              <div class="form-group">
                <label class="form-label">默认 PGID</label>
                <input v-model.number="deploymentPolicyForm.pgid" type="number" min="0" class="form-input" />
              </div>
            </div>
            <div class="form-group">
              <label class="form-label">默认时区</label>
              <input v-model.trim="deploymentPolicyForm.timezone" class="form-input" placeholder="Asia/Shanghai" />
            </div>
            <div class="nas-library-grid">
              <div class="form-group">
                <label class="form-label">媒体目录</label>
                <input v-model.trim="deploymentPolicyForm.mediaPath" class="form-input" placeholder="/volume1/media" />
              </div>
              <div class="form-group">
                <label class="form-label">照片目录</label>
                <input v-model.trim="deploymentPolicyForm.photoPath" class="form-input" placeholder="/volume1/photo" />
              </div>
              <div class="form-group">
                <label class="form-label">音乐目录</label>
                <input v-model.trim="deploymentPolicyForm.musicPath" class="form-input" placeholder="/volume1/music" />
              </div>
              <div class="form-group">
                <label class="form-label">漫画目录</label>
                <input v-model.trim="deploymentPolicyForm.comicPath" class="form-input" placeholder="/volume1/comic" />
              </div>
              <div class="form-group">
                <label class="form-label">小说目录</label>
                <input v-model.trim="deploymentPolicyForm.novelPath" class="form-input" placeholder="/volume1/novel" />
              </div>
            </div>
            <p class="form-hint">填写宿主机上的标准媒体库目录，部署 Jellyfin 等应用时可直接复用；留空的类型不会参与目录映射。</p>
            <div class="form-group">
              <SwitchToggle v-model="deploymentPolicyForm.allowHost" label="自动允许 Host 网络" />
              <p class="form-hint">只允许保留上游模板自带的 Host 网络配置，不会把所有应用改为 Host 网络</p>
            </div>
            <div class="form-actions">
              <button
                v-ripple
                class="btn btn-primary"
                :class="{ 'is-loading': deploymentPolicySaving }"
                :disabled="deploymentPolicyLoading || deploymentPolicySaving"
                @click="saveDeploymentPolicy"
              >
                <span v-if="deploymentPolicySaving" class="btn-spinner"></span>
                保存默认值
              </button>
            </div>
          </div>
        </div>

        <div
          v-if="isSettingSectionVisible('remoteNodes')"
          id="remote-management-settings"
          class="settings-card"
          :class="{ 'settings-column-start': isSettingColumnStart('remoteNodes') }"
          data-setting-key="remoteNodes"
        >
          <RemoteNodesSettings :go-enabled="licenseStatus.tier === 'go'" />
        </div>

        <!-- AI 助手 -->
        <div
          v-if="isSettingSectionVisible('ai')"
          id="ai-settings"
          class="settings-card"
          data-setting-key="ai"
        >
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="ai" :size="18" class="card-icon" />
              AI 配置中心
            </h3>
          </div>
          <div class="card-body">
            <details class="setting-disclosure" data-settings-group="ai-connection" open>
              <summary class="setting-disclosure__summary">
                <span><strong>连接与授权</strong><small>服务地址、密钥与可用模型</small></span>
                <DynamicIcon name="chevron-down" :size="16" />
              </summary>
              <div class="setting-disclosure__body">
            <div class="form-group">
              <SwitchToggle v-model="settingsForm.aiEnabled" label="启用 AI 能力" />
              <p class="form-hint">{{ editionSettingsCopy.aiEnabledHint }}</p>
            </div>
            <EditionSettingsFields v-if="isFullEdition" v-model:fields="editionFields" section="ai-connection" />
            <div class="form-group">
              <label class="form-label">统一 Base URL</label>
              <input v-model="settingsForm.aiBaseUrl" class="form-input" placeholder="例如：https://api.deepseek.com/v1 / https://api.minimax.chat/v1 / https://api.moonshot.cn/v1" />
              <p class="form-hint">后端不会自动补全版本路径；诊断会检查 /models，正式请求使用 /chat/completions</p>
              <p class="form-hint">最终请求 URL：{{ aiFinalUrl || '-' }}</p>
            </div>
            <div class="form-group">
              <label class="form-label">统一 API Key</label>
              <input v-model="settingsForm.aiApiKey" type="password" class="form-input" placeholder="留空表示不修改" />
              <p class="form-hint">{{ editionSettingsCopy.aiKeyHint }}</p>
              <p class="form-hint">当前状态：{{ aiKeyStatusText }}。出于安全考虑，不会回显已保存的 Key</p>
            </div>
            <div class="form-group">
              <label class="form-label">可用模型</label>
              <div class="ai-model-fetch">
                <button
                  v-ripple
                  type="button"
                  class="btn btn-default"
                  :class="{ 'is-loading': aiModelsLoading }"
                  :disabled="aiModelsLoading"
                  @click="fetchAiModels"
                >
                  <span v-if="aiModelsLoading" class="btn-spinner"></span>
                  获取模型列表
                </button>
                <span v-if="aiModelsCache" class="form-hint">已缓存 {{ aiModelsCache.models.length }} 个模型</span>
              </div>
              <p v-if="aiModelsError" class="form-hint ai-models-error">{{ aiModelsError }}</p>
              <p v-else-if="aiModelsBaseUrlMismatch" class="form-hint">当前 Base URL 与缓存来源（{{ aiModelsCache.baseUrl }}）不一致，请重新获取模型列表</p>
              <p class="form-hint">拉取成功即表示连接正常；列表缓存在本浏览器，作为下方两个模型字段的下拉候选</p>
            </div>
              </div>
            </details>
            <details class="setting-disclosure" data-settings-group="ai-models">
              <summary class="setting-disclosure__summary">
                <span><strong>模型配置</strong><small>主模型、轻量模型与生成参数</small></span>
                <DynamicIcon name="chevron-down" :size="16" />
              </summary>
              <div class="setting-disclosure__body">
            <div class="form-group">
              <label class="form-label">{{ editionSettingsCopy.aiModelLabel }}</label>
              <SearchableSelect
                v-model="settingsForm.aiModel"
                :options="aiModelSelectOptions"
                search-placeholder="搜索模型或输入自定义模型 ID..."
                placeholder="例如：deepseek-v4-flash / MiniMAX-M3 / K2.7 / glm-5.2"
                @select="autoSaveAiSettings"
              />
              <p class="form-hint">从列表中选择后立即自动保存；也可以输入列表外的模型 ID</p>
            </div>
            <div class="form-group">
              <label class="form-label">轻量任务模型</label>
              <SearchableSelect
                v-model="settingsForm.aiUtilityModel"
                :options="aiModelSelectOptions"
                search-placeholder="搜索模型或输入自定义模型 ID..."
                placeholder="留空时使用主模型"
                clearable
                clear-label="留空（使用主模型）"
                @select="autoSaveAiSettings"
              />
              <p class="form-hint">{{ editionSettingsCopy.aiUtilityHint }}</p>
            </div>
            <div class="form-group">
              <label class="form-label">推荐配置</label>
              <div class="ai-preset-list">
                <button
                  v-ripple
                  v-for="preset in aiPresets"
                  :key="`${preset.name}-${preset.endpointLabel}`"
                  type="button"
                  class="ai-preset-card"
                  @click="applyAiPreset(preset)"
                >
                  <span class="ai-preset-name">{{ preset.name }}</span>
                  <span class="ai-preset-endpoint">{{ preset.endpointLabel }}</span>
                  <span class="ai-preset-model">{{ preset.model }}</span>
                </button>
              </div>
            </div>
            <div class="form-group">
              <label class="form-label">Temperature</label>
              <input v-model.number="settingsForm.aiTemperature" type="number" min="0" max="2" step="0.1" class="form-input" />
            </div>
            <div v-if="isFullEdition" class="form-group">
              <label class="form-label">Max Tokens</label>
              <input v-model.number="settingsForm.aiMaxTokens" type="number" min="256" max="16000" step="256" class="form-input" />
              <p class="form-hint">模型默认最大输出长度，推荐 8192；不同场景会使用自己的上限</p>
            </div>
              </div>
            </details>
            <details class="setting-disclosure" data-settings-group="ai-behavior">
              <summary class="setting-disclosure__summary">
                <span><strong>行为与提示词</strong><small>{{ editionSettingsCopy.aiBehaviorSubtitle }}</small></span>
                <DynamicIcon name="chevron-down" :size="16" />
              </summary>
              <div class="setting-disclosure__body">
            <div class="form-group">
              <SwitchToggle v-model="settingsForm.aiAllowCreateCategory" label="允许 AI 创建导航分类" />
              <p class="form-hint">关闭后，AI 只能从现有分类候选中选择；开启后可创建简短、可复用的新分类</p>
            </div>
            <div class="form-group">
              <label class="form-label">导航识别提示词</label>
              <textarea v-model="settingsForm.aiNavigationPrompt" rows="4" class="form-textarea" placeholder="用于导航标题、分类、图标识别"></textarea>
              <p class="form-hint">场景：导航页重新识别、容器自动发现后的 AI 补全</p>
            </div>
            <div class="form-group">
              <label class="form-label">Compose 生成提示词</label>
              <textarea v-model="settingsForm.aiComposePrompt" rows="4" class="form-textarea" placeholder="用于 Compose YAML 和 .env 生成"></textarea>
              <p class="form-hint">{{ editionSettingsCopy.aiComposePromptHint }}</p>
            </div>
            <EditionSettingsFields v-if="isFullEdition" v-model:fields="editionFields" section="ai-behavior" />
              </div>
            </details>
            <div class="form-actions">
              <button v-ripple class="btn btn-primary" :class="{ 'is-loading': aiSaving }" :disabled="aiSaving || aiClearing" @click="saveAiSettings">
                <span v-if="aiSaving" class="btn-spinner"></span>
                保存 AI 配置
              </button>
              <button v-ripple class="btn btn-default" :class="{ 'is-loading': aiTesting }" :disabled="aiTesting" @click="testAiConnectivity">
                <span v-if="aiTesting" class="btn-spinner"></span>
                连接性测试
              </button>
              <button v-if="settingsForm.aiApiKeySet" v-ripple class="btn btn-danger" :class="{ 'is-loading': aiClearing }" :disabled="aiSaving || aiClearing" @click="clearAiApiKey">
                <span v-if="aiClearing" class="btn-spinner"></span>
                清空 Key
              </button>
            </div>
            <div v-if="aiTestResult" class="alert" :class="aiTestResult.ok ? 'alert-success' : 'alert-danger'">
              <DynamicIcon :name="aiTestResult.ok ? 'success' : 'warning'" :size="16" />
              <div>
                <div>{{ aiTestResult.message }}</div>
                <div v-if="aiTestResult.detail" class="alert-detail">{{ aiTestResult.detail }}</div>
              </div>
            </div>
          </div>
        </div>

        <LicenseSettingsPanel
          v-if="isSettingSectionVisible('license')"
          id="license-settings"
          class="settings-card"
          :class="{ 'settings-column-start': isSettingColumnStart('license') }"
          data-setting-key="license"
          :status="licenseStatus"
          @status-change="onLicenseStatusChange"
        />

        <!-- 通知偏好 -->
        <div
          v-if="isSettingSectionVisible('notifications')"
          id="notification-settings"
          class="settings-card"
          data-setting-key="notifications"
        >
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="bell" :size="18" class="card-icon" />
              通知偏好
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <label class="form-label">保留通知分类</label>
              <div class="checkbox-grid">
                <AnimatedCheckbox
                  v-for="item in notificationCategoryOptions"
                  :key="item.value"
                  v-model="settingsForm.notificationEnabledCategories"
                  :value="item.value"
                  :label="item.label"
                />
              </div>
              <p class="form-hint">未勾选的类型不会再产生新的通知；历史通知仍保留在数据库日志中</p>
            </div>
            <div class="form-actions">
              <button
                v-ripple
                class="btn btn-primary"
                :class="{ 'is-loading': notificationSaving }"
                :disabled="notificationSaving"
                @click="saveNotificationSettings"
              >
                <span v-if="notificationSaving" class="btn-spinner"></span>
                保存通知偏好
              </button>
            </div>
            <div class="notification-channels">
              <div class="notification-channels-header">
                <div>
                  <label class="form-label">外部通知通道</label>
                  <p class="form-hint">支持 Webhook、ntfy、Gotify、Bark 和 PushPlus；失败仅记录投递状态，不影响部署或备份。</p>
                </div>
                <button type="button" class="btn btn-default notification-add-btn" @click="openNotificationChannelDialog()">
                  <DynamicIcon name="plus" :size="15" />
                  添加通道
                </button>
              </div>
              <div v-if="notificationChannels.length" class="notification-channel-list">
                <div v-for="channel in notificationChannels" :key="channel.id" class="notification-channel-row">
                  <div class="notification-channel-main">
                    <strong>{{ channel.name }}</strong>
                    <span>{{ notificationChannelTypeLabel(channel.type) }} · {{ notificationCategorySummary(channel.categories) }}</span>
                  </div>
                  <div class="notification-channel-actions">
                    <span class="notification-channel-state" :class="{ muted: !channel.enabled }">{{ channel.enabled ? '已启用' : '已停用' }}</span>
                    <button type="button" class="table-btn info" title="测试通知" :disabled="notificationChannelTesting === channel.id" @click="testNotificationChannel(channel)">
                      <DynamicIcon name="send" :size="14" />
                    </button>
                    <button type="button" class="table-btn info" title="编辑" @click="openNotificationChannelDialog(channel)">
                      <DynamicIcon name="edit" :size="14" />
                    </button>
                    <button type="button" class="table-btn info" title="投递记录" @click="openNotificationDeliveryDialog(channel)">
                      <DynamicIcon name="list" :size="14" />
                    </button>
                    <button type="button" class="table-btn danger" title="删除" @click="removeNotificationChannel(channel)">
                      <DynamicIcon name="trash-2" :size="14" />
                    </button>
                  </div>
                </div>
              </div>
              <p v-else class="form-hint notification-channel-empty">尚未配置外部通知通道。</p>
            </div>
          </div>
        </div>

        <GitHubAppSettingsPanel
          v-if="licenseStatus.tier === 'go' && isSettingSectionVisible('github')"
          id="github-token-settings"
          class="settings-card"
          :class="{ 'settings-column-start': isSettingColumnStart('github') }"
          data-setting-key="github"
        />

        <!-- 端口管理设置 -->
        <div
          v-if="isSettingSectionVisible('ports')"
          id="port-settings"
          class="settings-card"
          :class="{ 'settings-column-start': isSettingColumnStart('ports') }"
          data-setting-key="ports"
        >
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="network" :size="18" class="card-icon" />
              端口管理设置
            </h3>
          </div>
          <div class="card-body">
            <div class="form-group">
              <label class="form-label">自动分配范围</label>
              <div class="range-inputs">
                <input v-model.number="allocSettings.start" type="number" min="1024" max="65535" class="form-input" placeholder="起始端口" />
                <span class="range-separator">-</span>
                <input v-model.number="allocSettings.end" type="number" min="1024" max="65535" class="form-input" placeholder="结束端口" />
              </div>
              <p class="form-hint">供自动部署工具调用分配接口时使用；AppStore 当前不会自动覆盖未修改的默认端口</p>
            </div>
            <div class="form-group">
              <SwitchToggle v-model="allocSettings.allowAutoAllocPort" label="允许自动部署工具分配端口" />
              <p class="form-hint">开启后自动部署工具才可从上述范围分配并预留端口；端口页面不再提供无消费场景的手动预留入口</p>
            </div>
            <div class="form-actions">
              <button v-ripple class="btn btn-primary" :class="{ 'is-loading': allocLoading }" :disabled="allocLoading" @click="saveAllocSettings">
                <span v-if="allocLoading" class="btn-spinner"></span>
                保存范围
              </button>
            </div>
          </div>
        </div>

        <!-- 卷备份 -->
        <div
          v-if="isSettingSectionVisible('backup')"
          id="volume-backup-settings"
          class="settings-card"
          :class="{ 'settings-column-start': isSettingColumnStart('backup') }"
          data-setting-key="backup"
        >
          <div class="card-header">
            <h3 class="card-title">
              <DynamicIcon name="backup" :size="18" class="card-icon" />
              卷备份（docker-volume-backup）
            </h3>
          </div>
          <div class="card-body">
            <details class="setting-disclosure" data-settings-group="backup-local" open>
              <summary class="setting-disclosure__summary">
                <span><strong>基础与本地备份</strong><small>卷、归档目录与执行计划</small></span>
                <DynamicIcon name="chevron-down" :size="16" />
              </summary>
              <div class="setting-disclosure__body">
            <div class="form-group">
              <SwitchToggle v-model="settingsForm.volumeBackupEnabled" label="启用卷定时备份" />
              <p class="form-hint">启用后会创建并托管一个 offen/docker-volume-backup 容器，配置来自下方环境变量</p>
            </div>
            <div class="form-group">
              <label class="form-label">镜像</label>
              <input v-model="settingsForm.volumeBackupImage" class="form-input" placeholder="offen/docker-volume-backup:latest" />
            </div>
            <div class="form-group">
              <label class="form-label">选择需要备份的卷</label>
              <Multiselect
                v-model="settingsForm.volumeBackupVolumes"
                :options="volumeOptions"
                placeholder="选择需要备份的卷（可多选）"
              />
              <p class="form-hint">会以只读方式挂载到 /backup/&lt;volume&gt; 供 docker-volume-backup 备份</p>
            </div>
            <div class="form-group">
              <label class="form-label">本地归档目录（可选）</label>
              <input v-model="settingsForm.volumeBackupArchiveDir" class="form-input" placeholder="例如：/data/backups" />
              <p class="form-hint">配置后会将该宿主机绝对路径挂载到容器 /archive，用于保存本地备份副本；目录不存在时会自动创建</p>
              <div class="alert alert-warning">
                <DynamicIcon name="warning" :size="16" />
                请确认该路径位于 Docker 守护进程所在宿主机，且有足够磁盘空间
              </div>
            </div>
            <div class="form-group">
              <label class="form-label">每日备份（Cron 表达式）</label>
              <input v-model="settingsForm.volumeBackupCronExpression" class="form-input" placeholder="@daily" />
              <p class="form-hint">默认 @daily。更多配置参考：https://offen.github.io/docker-volume-backup/reference/</p>
            </div>
            <div class="form-group">
              <SwitchToggle v-model="settingsForm.volumeBackupMountDockerSock" label="挂载 Docker Socket" />
              <p class="form-hint">允许备份容器与 Docker 交互（例如 stop-during-backup）。禁用则不挂载 /var/run/docker.sock</p>
            </div>
              </div>
            </details>
            <details class="setting-disclosure" data-settings-group="backup-remote">
              <summary class="setting-disclosure__summary">
                <span><strong>远程存储</strong><small>S3、WebDAV 与高级环境变量</small></span>
                <DynamicIcon name="chevron-down" :size="16" />
              </summary>
              <div class="setting-disclosure__body">
            <div class="form-group">
              <label class="form-label">远程存储预设</label>
              <div class="inline-field">
                <select v-model="volumeBackupStoragePreset" class="form-input">
                  <option value="s3">S3 兼容存储</option>
                  <option value="webdav">WebDAV</option>
                </select>
                <button type="button" class="btn btn-secondary" @click="applyVolumeBackupStoragePreset">
                  写入模板
                </button>
              </div>
              <p class="form-hint">会把 S3/WebDAV 所需变量追加或更新到下方环境变量；保存后重建备份容器即可生效</p>
            </div>
            <div class="form-group">
              <label class="form-label">环境变量</label>
              <textarea v-model="settingsForm.volumeBackupEnv" rows="6" class="form-textarea" placeholder="留空表示不修改；重新输入会覆盖已保存配置。例如：&#10;BACKUP_CRON_EXPRESSION=0 3 * * *&#10;BACKUP_FILENAME=backup-%Y-%m-%dT%H-%M-%S.tar.gz&#10;AWS_S3_BUCKET_NAME=xxx"></textarea>
              <p v-if="settingsForm.volumeBackupEnvStored" class="form-hint">已加密保存。出于安全考虑不会回显，留空表示不修改。</p>
              <p v-else class="form-hint">仅支持 KEY=VALUE 格式；不会在界面日志中回显敏感值</p>
            </div>
              </div>
            </details>
            <div class="form-actions">
              <button v-ripple class="btn btn-primary" :class="{ 'is-loading': volumeBackupSaving }" :disabled="volumeBackupSaving" @click="saveVolumeBackupSettings">
                <span v-if="volumeBackupSaving" class="btn-spinner"></span>
                保存卷备份配置
              </button>
              <button
                v-ripple
                class="btn btn-warning" 
                :class="{ 'is-loading': volumeBackupRebuilding }"
                :disabled="!settingsForm.volumeBackupEnabled || volumeBackupRebuilding" 
                @click="rebuildVolumeBackup"
              >
                <span v-if="volumeBackupRebuilding" class="btn-spinner"></span>
                重建备份容器
              </button>
              <button v-ripple class="btn btn-default" @click="refreshVolumeOptions">刷新卷列表</button>
            </div>
          </div>
        </div>

    </div>
    </div>
  </ResourceWorkbench>

  <Modal v-model:visible="notificationChannelDialogVisible" :title="editingNotificationChannel ? '编辑通知通道' : '添加通知通道'" width="520px">
    <div class="form-group">
      <label class="form-label">名称</label>
      <input v-model.trim="notificationChannelForm.name" class="form-input" placeholder="例如：手机通知" />
    </div>
    <div class="form-group">
      <label class="form-label">通道类型</label>
      <select v-model="notificationChannelForm.type" class="form-input">
        <option value="webhook">Webhook</option>
        <option value="wecom">企业微信机器人</option>
        <option value="ntfy">ntfy</option>
        <option value="gotify">Gotify</option>
        <option value="bark">Bark</option>
        <option value="pushplus">PushPlus（微信/企业微信）</option>
      </select>
    </div>
    <div class="form-group">
      <SwitchToggle v-model="notificationChannelForm.enabled" label="启用此通道" />
    </div>
    <template v-if="notificationChannelForm.type === 'webhook'">
      <div class="form-group"><label class="form-label">Webhook 地址</label><input v-model.trim="notificationChannelForm.url" class="form-input" placeholder="https://example.com/tradis" /></div>
      <div class="form-group"><label class="form-label">签名密钥</label><input v-model="notificationChannelForm.signingSecret" type="password" class="form-input" :placeholder="notificationChannelSecretPlaceholder" /></div>
    </template>
    <template v-else-if="notificationChannelForm.type === 'wecom'">
      <div class="form-group">
        <label class="form-label">机器人 Key</label>
        <input v-model="notificationChannelForm.wecomKey" type="password" class="form-input" :placeholder="notificationChannelSecretPlaceholder" />
        <p class="form-hint">从完整机器人 URL 中填写 key= 后的值。</p>
      </div>
      <div class="form-group"><label class="form-label">服务地址（可选，默认官方）</label><input v-model.trim="notificationChannelForm.server" class="form-input" placeholder="https://qyapi.weixin.qq.com" /></div>
    </template>
    <template v-else-if="notificationChannelForm.type === 'ntfy'">
      <div class="form-group"><label class="form-label">服务地址</label><input v-model.trim="notificationChannelForm.server" class="form-input" placeholder="https://ntfy.sh" /></div>
      <div class="form-group"><label class="form-label">Topic</label><input v-model.trim="notificationChannelForm.topic" class="form-input" placeholder="tradis" /></div>
      <div class="form-group"><label class="form-label">访问 Token</label><input v-model="notificationChannelForm.token" type="password" class="form-input" :placeholder="notificationChannelSecretPlaceholder" /></div>
    </template>
    <template v-else-if="notificationChannelForm.type === 'gotify'">
      <div class="form-group"><label class="form-label">服务地址</label><input v-model.trim="notificationChannelForm.server" class="form-input" placeholder="https://gotify.example" /></div>
      <div class="form-group"><label class="form-label">应用 Token</label><input v-model="notificationChannelForm.token" type="password" class="form-input" :placeholder="notificationChannelSecretPlaceholder" /></div>
    </template>
    <template v-else-if="notificationChannelForm.type === 'bark'">
      <div class="form-group"><label class="form-label">服务地址</label><input v-model.trim="notificationChannelForm.server" class="form-input" placeholder="https://api.day.app" /></div>
      <div class="form-group"><label class="form-label">设备密钥</label><input v-model="notificationChannelForm.deviceKey" type="password" class="form-input" :placeholder="notificationChannelSecretPlaceholder" /></div>
    </template>
    <template v-else-if="notificationChannelForm.type === 'pushplus'">
      <div class="form-group"><label class="form-label">Token</label><input v-model="notificationChannelForm.token" type="password" class="form-input" :placeholder="notificationChannelSecretPlaceholder" /></div>
      <div class="form-group"><label class="form-label">推送渠道（可选）</label>
        <select v-model.trim="notificationChannelForm.pushChannel" class="form-input">
          <option value="">微信公众号（默认）</option>
          <option value="cp">企业微信应用</option>
          <option value="webhook">企业微信群机器人</option>
        </select>
      </div>
      <div v-if="pushPlusOptionRequired(notificationChannelForm.pushChannel)" class="form-group">
        <label class="form-label">{{ pushPlusOptionLabel(notificationChannelForm.pushChannel) }}</label>
        <input v-model.trim="notificationChannelForm.pushOption" class="form-input" :placeholder="pushPlusOptionLabel(notificationChannelForm.pushChannel)" />
        <p class="form-hint">
          <template v-if="!notificationChannelForm.pushOption">需要补充{{ pushPlusOptionLabel(notificationChannelForm.pushChannel) }}；</template>该编码在 PushPlus 个人中心维护，不是机器人 Key 或 URL。
        </p>
      </div>
      <div class="form-group"><label class="form-label">服务地址（可选，默认官方）</label><input v-model.trim="notificationChannelForm.server" class="form-input" placeholder="https://www.pushplus.plus" /></div>
    </template>
    <div class="form-group">
      <label class="form-label">通知分类</label>
      <div class="checkbox-grid">
        <AnimatedCheckbox
          v-for="item in notificationCategoryOptions"
          :key="item.value"
          v-model="notificationChannelForm.categories"
          :value="item.value"
          :label="item.label"
        />
      </div>
    </div>
    <div v-if="editingNotificationChannel?.secretSet" class="form-group">
      <AnimatedCheckbox v-model="notificationChannelForm.clearSecrets" label="清除已保存的密钥" />
    </div>
    <template #footer>
      <button class="btn btn-default" :disabled="notificationChannelSaving" @click="notificationChannelDialogVisible = false">取消</button>
      <button
        class="btn btn-primary"
        :class="{ 'is-loading': notificationChannelSaving }"
        :disabled="notificationChannelSaving || notificationChannelSaveBlocked"
        :title="notificationChannelSaveBlocked ? '请先补充 PushPlus 渠道编码' : ''"
        @click="saveNotificationChannel"
      >保存通道</button>
    </template>
  </Modal>

  <Modal v-model:visible="notificationDeliveryDialogVisible" :title="`${notificationDeliveryChannel?.name || ''} · 投递记录`" width="680px">
    <div v-if="notificationDeliveries.length" class="notification-delivery-list">
      <div v-for="delivery in notificationDeliveries" :key="delivery.id" class="notification-delivery-row">
        <div>
          <strong>{{ notificationDeliveryStatusLabel(delivery.status) }}</strong>
          <span>{{ notificationCategorySummary([delivery.eventCategory]) }} · {{ delivery.eventType || 'general' }} · {{ delivery.attempts }} 次尝试</span>
          <small v-if="delivery.lastError">{{ delivery.lastError }}</small>
        </div>
        <time>{{ delivery.createdAt ? formatTime(delivery.createdAt) : '-' }}</time>
      </div>
    </div>
    <p v-else class="form-hint">暂无投递记录。</p>
    <template #footer><button class="btn btn-default" @click="notificationDeliveryDialogVisible = false">关闭</button></template>
  </Modal>
</template>

<script setup>
import { storeToRefs } from 'pinia'
import { ref, computed, nextTick, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import auth from '@/api/auth.js'
import system from '@/api/system.js'
import volumes from '@/api/volumes.js'
import settings from '@/api/settings.js'
import Multiselect from '@/components/ui/Multiselect.vue'
import SearchableSelect from '@/components/ui/SearchableSelect.vue'
import DynamicIcon from '@/components/ui/DynamicIcon.vue'
import Modal from '@/components/feedback/Modal.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import SegmentedTabs from '@/components/ui/SegmentedTabs.vue'
import SwitchToggle from '@/components/ui/SwitchToggle.vue'
import AnimatedCheckbox from '@/components/ui/AnimatedCheckbox.vue'
import ResourceWorkbench from '@/components/resource-workbench/ResourceWorkbench.vue'
import { useThemeStore, themes as colorThemes } from '@/stores/theme.js'
import { useIconStore } from '@/stores/icons.js'
import { useToast } from '@/composables/useToast.js'
import { useUiStore } from '@/stores/ui.js'
import { useOnboardingStore } from '@/stores/onboarding.js'
import {
  buildPushPlusConfig,
  normalizePushPlusChannel,
  pushPlusOptionLabel,
  pushPlusOptionMissing,
  pushPlusOptionRequired
} from './notificationChannelForm.js'
import {
  SETTINGS_CATEGORY_STORAGE_KEY,
  settingCategories,
  settingSections,
  getVisibleSettingSectionKeys,
  resolveSettingSectionTarget,
  resolveStoredSettingCategory
} from './settingsSections.js'
import { formatAIDiagnosticResult } from './aiDiagnostics.js'
import {
  RemoteNodesSettings,
  LicenseSettingsPanel,
  GitHubAppSettingsPanel,
  EditionSettingsFields,
  editionSettingsCopy,
  createEditionSettingsFields,
  readEditionSettingsFields,
  getEditionServerFields,
  getEditionAiFields,
  loadEditionLicenseStatus,
  loadFullSettingsDependencies as loadEditionSettingsDependencies
} from '@edition/settings-dependencies'
import {
  readAiModelsCache,
  writeAiModelsCache,
  buildModelSelectOptions,
  isCacheBaseUrlMismatch
} from './aiModelPicker.js'
import { buildDeploymentPolicyUpdate, createDeploymentPolicyForm } from './nasProfile.js'
import { formatTime } from '@/utils/format.js'

const RECOMMENDED_AI_MAX_TOKENS = 8192
const isFullEdition = __TRADIS_EDITION__ === 'full'

let aiApi = null
let testAI = null

async function loadFullSettingsDependencies() {
  const dependencies = await loadEditionSettingsDependencies()
  aiApi = dependencies.aiApi
  testAI = dependencies.testAI
}

const toast = useToast()
const uiStore = useUiStore()
const onboarding = useOnboardingStore()
const route = useRoute()
const router = useRouter()

function ensureAISettingsReady() {
  if (aiApi && typeof testAI === 'function') return true
  toast.error('AI 设置暂不可用，请刷新页面后重试')
  return false
}

function onToggleOnboardingBanner(enabled) {
  if (enabled) {
    onboarding.reset()
    toast.success('已重新开启新手引导横幅')
  } else {
    onboarding.skip()
    toast.success('已关闭新手引导横幅')
  }
}

// 主题 store
const themeStore = useThemeStore()
const { isDark, currentTheme } = storeToRefs(themeStore)
const { toggleDarkMode, setTheme } = themeStore

// 图标 store
const iconStore = useIconStore()

// 图标集选项
const iconSets = [
  { id: 'lucide', name: 'Lucide' },
  { id: 'tabler', name: 'Tabler' }
]

const settingsSearchQuery = ref('')
const activeSettingCategory = ref(resolveStoredSettingCategory(localStorage.getItem(SETTINGS_CATEGORY_STORAGE_KEY)))
const settingCategoryTabs = settingCategories.map(category => ({
  value: category.key,
  label: category.label
}))
const settingsPageRef = ref(null)
const settingsMounted = ref(false)

const visibleSettingSections = computed(() => getVisibleSettingSectionKeys({
  activeCategory: activeSettingCategory.value,
  query: settingsSearchQuery.value,
  isGo: licenseStatus.value.tier === 'go'
}))

function isSettingSectionVisible(key) {
  return visibleSettingSections.value.has(key)
}

function isSettingColumnStart(key) {
  const section = settingSections.find((item) => item.key === key)
  if (section?.column !== 'right' || !visibleSettingSections.value.has(key)) return false

  const visibleLeft = settingSections.some((item) => (
    item.column === 'left' &&
    visibleSettingSections.value.has(item.key)
  ))
  if (!visibleLeft) return false

  const firstVisibleRight = settingSections.find((item) => (
    item.column === 'right' &&
    visibleSettingSections.value.has(item.key)
  ))
  return firstVisibleRight?.key === key
}

function selectSettingCategory(categoryKey) {
  if (!settingCategories.some((category) => category.key === categoryKey)) return
  activeSettingCategory.value = categoryKey
  settingsSearchQuery.value = ''
  settingsPageRef.value?.scrollTo({ top: 0, behavior: 'auto' })
}

async function scrollSettingSectionIntoView(sectionKey) {
  const target = resolveSettingSectionTarget(sectionKey)
  if (!target) return

  activeSettingCategory.value = target.category
  settingsSearchQuery.value = ''
  await nextTick()
  document.getElementById(target.elementId)?.scrollIntoView({ block: 'start', behavior: 'smooth' })

  const query = { ...route.query }
  delete query.section
  await router.replace({ query })
}

watch(
  () => route.query?.section,
  sectionKey => {
    if (settingsMounted.value && sectionKey) void scrollSettingSectionIntoView(sectionKey)
  }
)

watch(activeSettingCategory, category => {
  localStorage.setItem(SETTINGS_CATEGORY_STORAGE_KEY, category)
})

const defaultNotificationCategories = ['deploy_task', 'git_task', 'navigation_task', 'volume_backup_task', 'app_protection_task', 'system']
const notificationCategoryOptions = [
  { value: 'deploy_task', label: '部署任务' },
  { value: 'git_task', label: 'Git 任务' },
  { value: 'navigation_task', label: '导航任务' },
  { value: 'volume_backup_task', label: '卷备份任务' },
  { value: 'app_protection_task', label: '应用保护任务' },
  { value: 'system', label: '系统通知' }
]

const notificationChannelTypeOptions = {
  webhook: 'Webhook',
  wecom: '企业微信机器人',
  ntfy: 'ntfy',
  gotify: 'Gotify',
  bark: 'Bark',
  pushplus: 'PushPlus'
}

function createNotificationChannelForm(channel = null) {
  const config = channel?.config || {}
  const rawPushChannel = String(config.channel || '').trim()
  return {
    name: channel?.name || '',
    type: channel?.type || 'webhook',
    enabled: typeof channel?.enabled === 'boolean' ? channel.enabled : true,
    categories: Array.isArray(channel?.categories) && channel.categories.length
      ? [...channel.categories]
      : [...defaultNotificationCategories],
    url: config.url || '',
    server: config.server || '',
    topic: config.topic || '',
    token: '',
    signingSecret: '',
    wecomKey: '',
    deviceKey: '',
    // 历史无效渠道值由 normalizePushPlusChannel 归一化为 webhook；
    // 其编码语义已失效，必须重新填写，不回填。
    pushChannel: normalizePushPlusChannel(rawPushChannel),
    pushOption: rawPushChannel === 'cpwebhook' ? '' : (config.option || ''),
    clearSecrets: false,
    originalType: channel?.type || 'webhook'
  }
}

const volumeBackupStorageTemplates = {
  s3: [
    'AWS_S3_BUCKET_NAME=',
    'AWS_S3_PATH=tradis-volume-backups',
    'AWS_ACCESS_KEY_ID=',
    'AWS_SECRET_ACCESS_KEY=',
    'AWS_ENDPOINT=s3.amazonaws.com',
    'AWS_ENDPOINT_PROTO=https',
    'AWS_ENDPOINT_INSECURE=false'
  ],
  webdav: [
    'WEBDAV_URL=',
    'WEBDAV_PATH=/tradis-volume-backups/',
    'WEBDAV_USERNAME=',
    'WEBDAV_PASSWORD=',
    'WEBDAV_URL_INSECURE=false'
  ]
}

// 设置图标集
const setIconSet = (set) => {
  iconStore.setIconSet(set)
}

// 设置表单
const settingsForm = ref({
  lanUrl: '',
  wanUrl: '',
  appStoreCDNURL: '',
  advancedMode: false,
  imageUpdateIntervalMinutes: 120,
  aiEnabled: false,
  aiBaseUrl: '',
  aiApiKey: '',
  aiApiKeySet: false,
  aiApiKeyStored: false,
  aiApiKeySource: '',
  aiModel: '',
  aiUtilityModel: '',
  aiTemperature: 0.7,
  aiMaxTokens: RECOMMENDED_AI_MAX_TOKENS,
  aiAllowCreateCategory: true,
  aiNavigationPrompt: '',
  aiComposePrompt: '',
  volumeBackupEnabled: false,
  volumeBackupImage: 'offen/docker-volume-backup:latest',
  volumeBackupEnv: '',
  volumeBackupEnvSet: false,
  volumeBackupEnvStored: false,
  volumeBackupCronExpression: '@daily',
  volumeBackupVolumes: [],
  volumeBackupArchiveDir: '',
  volumeBackupMountDockerSock: true,
  notificationEnabledCategories: [...defaultNotificationCategories]
})
const editionFields = ref(createEditionSettingsFields())
const fullServiceFieldsRef = ref(null)

// 密码表单
const passwordForm = ref({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

// 端口分配设置
const allocSettings = ref({
  start: 50000,
  end: 51000,
  allowAutoAllocPort: false
})

// 卷选项
const volumeOptions = ref([])
const volumeBackupStoragePreset = ref('s3')
const deploymentPolicyForm = ref(createDeploymentPolicyForm())

// 加载状态
const loading = ref(false)
const passwordLoading = ref(false)
const advancedModeLoading = ref(false)
const diagnosticExporting = ref(false)
const serverLoading = ref(false)
const deploymentPolicyLoading = ref(false)
const deploymentPolicySaving = ref(false)
const cdnRefreshing = ref(false)
const cdnStatus = ref(null)
const licenseStatus = ref({
  tier: 'free',
  state: 'free',
  features: [],
  installation_id: '',
  subscription_expires_at: '',
  last_error: ''
})
const aiSaving = ref(false)
const aiClearing = ref(false)
const aiTesting = ref(false)
const aiTestResult = ref(null)
// AI 模型列表：拉取结果只缓存在浏览器 localStorage，作为模型字段的下拉候选
const aiModelsLoading = ref(false)
const aiModelsError = ref('')
const aiModelsCache = ref(readAiModelsCache())
const volumeBackupSaving = ref(false)
const volumeBackupRebuilding = ref(false)
const notificationSaving = ref(false)
const notificationChannels = ref([])
const notificationChannelDialogVisible = ref(false)
const editingNotificationChannel = ref(null)
const notificationChannelSaving = ref(false)
const notificationChannelTesting = ref('')
const notificationChannelForm = ref(createNotificationChannelForm())

// cp/webhook 渠道的编码是投递硬依赖，缺失时禁止保存，避免保存出必失败的通道。
const notificationChannelSaveBlocked = computed(() =>
  notificationChannelForm.value.type === 'pushplus' &&
  pushPlusOptionMissing(notificationChannelForm.value)
)
const notificationDeliveryDialogVisible = ref(false)
const notificationDeliveryChannel = ref(null)
const notificationDeliveries = ref([])
const allocLoading = ref(false)

const notificationChannelSecretPlaceholder = computed(() => (
  editingNotificationChannel.value?.secretSet ? '留空表示保留已保存的密钥' : '可选'
))

const aiPresets = [
  {
    name: 'DeepSeek',
    endpointLabel: '官方 API 端点',
    baseURL: 'https://api.deepseek.com/v1',
    model: 'deepseek-v4-flash',
    temperature: 0.7,
    maxTokens: RECOMMENDED_AI_MAX_TOKENS
  },
  {
    name: 'MiniMax',
    endpointLabel: 'TokenPlan 端点',
    baseURL: 'https://api.minimax.chat/v1',
    model: 'MiniMAX-M3',
    temperature: 0.7,
    maxTokens: RECOMMENDED_AI_MAX_TOKENS
  },
  {
    name: 'Kimi',
    endpointLabel: 'Code Plan 端点',
    baseURL: 'https://api.moonshot.cn/v1',
    model: 'K2.7',
    temperature: 0.7,
    maxTokens: RECOMMENDED_AI_MAX_TOKENS
  },
  {
    name: 'GLM',
    endpointLabel: 'Code Plan 端点',
    baseURL: 'https://open.bigmodel.cn/api/paas/v4',
    model: 'glm-5.2',
    temperature: 0.7,
    maxTokens: RECOMMENDED_AI_MAX_TOKENS
  }
]

// AI 最终 URL
const aiFinalUrl = computed(() => {
  const base = (settingsForm.value.aiBaseUrl || '').trim().replace(/\/+$/, '')
  return base ? `${base}/chat/completions` : ''
})

const aiKeyStatusText = computed(() => {
  if (!settingsForm.value.aiApiKeySet) return '未配置'
  const stored = settingsForm.value.aiApiKeyStored ? '数据库已保存' : '数据库未保存'
  const source = settingsForm.value.aiApiKeySource || ''
  if (source.startsWith('env:')) {
    return `已配置（当前使用环境变量 ${source.replace('env:', '')}，${stored}）`
  }
  if (source === 'database') {
    return '已配置（当前使用数据库保存的 Key）'
  }
  return `已配置（${stored}）`
})

// 主模型/轻量模型下拉候选：缓存模型 + 当前已保存值（不在缓存时也可回显）
const aiModelSelectOptions = computed(() => (
  buildModelSelectOptions(aiModelsCache.value?.models || [], [
    settingsForm.value.aiModel,
    settingsForm.value.aiUtilityModel
  ])
))

// 表单 Base URL 与缓存来源不一致时提示重新拉取
const aiModelsBaseUrlMismatch = computed(() => (
  isCacheBaseUrlMismatch(aiModelsCache.value, settingsForm.value.aiBaseUrl)
))

const cdnStatusType = computed(() => {
  if (cdnStatus.value?.running) return 'info'
  if (cdnStatus.value?.bestIp) return 'success'
  if (cdnStatus.value?.lastError) return 'warning'
  return 'neutral'
})

const cdnStatusText = computed(() => {
  if (!settingsForm.value.appStoreCDNURL.trim()) return '配置 CDN 地址后可执行优选测速'
  if (cdnStatus.value?.running) return '正在重新测速，期间自动使用 CDN 域名访问'
  if (cdnStatus.value?.bestIp) return `当前优选 IP：${cdnStatus.value.bestIp}`
  if (cdnStatus.value?.lastError) return `优选 IP 不可用：${cdnStatus.value.lastError}`
  return '尚无优选结果，当前使用 CDN 域名访问'
})

const loadLicenseStatus = async () => {
  if (!isFullEdition) return
  try {
    const result = await loadEditionLicenseStatus()
    licenseStatus.value = { ...licenseStatus.value, ...(result || {}) }
  } catch (error) {
    console.error('加载许可证状态失败:', error)
  }
}

function onLicenseStatusChange(status) {
  licenseStatus.value = { ...licenseStatus.value, ...(status || {}) }
}

const formatCDNTestTime = (value) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

const loadCDNStatus = async () => {
  if (!isFullEdition) return
  try {
    cdnStatus.value = await system.getCDNStatus()
  } catch (error) {
    console.error('加载 Cloudflare 优选状态失败:', error)
  }
}

const waitForCDNRefresh = async () => {
  for (let i = 0; i < 180; i += 1) {
    await new Promise((resolve) => setTimeout(resolve, 1000))
    await loadCDNStatus()
    if (!cdnStatus.value?.running) return
  }
  throw new Error('测速仍在后台运行，请稍后查看状态')
}

const refreshCDNBestIP = async () => {
  if (!settingsForm.value.appStoreCDNURL.trim()) {
    toast.warning('请先填写并保存应用商店 CDN 地址')
    return
  }

  cdnRefreshing.value = true
  try {
    await system.refreshCDNSpeedTest()
    await loadCDNStatus()
    await waitForCDNRefresh()
    if (cdnStatus.value?.bestIp) {
      toast.success(`Cloudflare 优选 IP 已更新：${cdnStatus.value.bestIp}`)
    } else {
      throw new Error(cdnStatus.value?.lastError || '测速未产生可用 IP')
    }
  } catch (error) {
    console.error('刷新 Cloudflare 优选 IP 失败:', error)
    toast.error('刷新优选 IP 失败: ' + (error.message || '未知错误'))
    await loadCDNStatus()
  } finally {
    cdnRefreshing.value = false
  }
}

const applyAiPreset = (preset) => {
  settingsForm.value.aiBaseUrl = preset.baseURL
  settingsForm.value.aiModel = preset.model
  settingsForm.value.aiUtilityModel = ''
  settingsForm.value.aiTemperature = preset.temperature
  settingsForm.value.aiMaxTokens = preset.maxTokens
}

// 同步高级模式到 localStorage
const syncAdvancedModeLocal = () => {
  localStorage.setItem('advancedMode', settingsForm.value.advancedMode ? '1' : '0')
  window.dispatchEvent(new Event('advanced-mode-change'))
}

const loadDeploymentPolicy = async () => {
  deploymentPolicyLoading.value = true
  try {
    const response = await settings.getDeploymentDefaults()
    deploymentPolicyForm.value = createDeploymentPolicyForm(response?.profile || {})
  } catch (error) {
    console.error('加载 NAS 部署默认值失败:', error)
    toast.error('加载 NAS 部署默认值失败: ' + (error.message || '未知错误'))
  } finally {
    deploymentPolicyLoading.value = false
  }
}

const saveDeploymentPolicy = async () => {
  const payload = buildDeploymentPolicyUpdate(deploymentPolicyForm.value)
  if (!Number.isInteger(payload.puid) || payload.puid < 0 || !Number.isInteger(payload.pgid) || payload.pgid < 0) {
    toast.warning('PUID 和 PGID 必须是大于或等于 0 的整数')
    return
  }

  deploymentPolicySaving.value = true
  try {
    const response = await settings.updateDeploymentDefaults(payload)
    deploymentPolicyForm.value = createDeploymentPolicyForm(response?.profile || {})
    toast.success('NAS 部署默认值已保存')
  } catch (error) {
    console.error('保存 NAS 部署默认值失败:', error)
    toast.error('保存 NAS 部署默认值失败: ' + (error.message || '未知错误'))
  } finally {
    deploymentPolicySaving.value = false
  }
}

// 加载设置
const loadSettings = async () => {
  loading.value = true
  try {
    const res = await settings.getGlobal()
    if (res) {
      settingsForm.value = {
        ...settingsForm.value,
        lanUrl: res.lanUrl || '',
        wanUrl: res.wanUrl || '',
        appStoreCDNURL: res.appStoreCDNURL || '',
        advancedMode: !!res.advancedMode,
        imageUpdateIntervalMinutes: res.imageUpdateIntervalMinutes || 120,
        aiEnabled: !!res.aiEnabled,
        aiBaseUrl: res.aiBaseUrl || '',
        aiApiKey: '',
        aiApiKeySet: !!res.aiApiKeySet,
        aiApiKeyStored: !!res.aiApiKeyStored,
        aiApiKeySource: res.aiApiKeySource || '',
        aiModel: res.aiModel || '',
        aiUtilityModel: res.aiUtilityModel || '',
        aiTemperature: typeof res.aiTemperature === 'number' ? res.aiTemperature : 0.7,
        aiMaxTokens: Number(res.aiMaxTokens || RECOMMENDED_AI_MAX_TOKENS),
        aiAllowCreateCategory: typeof res.aiAllowCreateCategory === 'boolean' ? res.aiAllowCreateCategory : true,
        aiNavigationPrompt: res.aiNavigationPrompt || '',
        aiComposePrompt: res.aiComposePrompt || '',
        volumeBackupEnabled: !!res.volumeBackupEnabled,
        volumeBackupImage: res.volumeBackupImage || 'offen/docker-volume-backup:latest',
        volumeBackupEnv: '',
        volumeBackupEnvSet: !!res.volumeBackupEnvSet,
        volumeBackupEnvStored: !!res.volumeBackupEnvStored,
        volumeBackupCronExpression: res.volumeBackupCronExpression || '@daily',
        volumeBackupVolumes: Array.isArray(res.volumeBackupVolumes) ? res.volumeBackupVolumes : [],
        volumeBackupArchiveDir: res.volumeBackupArchiveDir || '',
        volumeBackupMountDockerSock: typeof res.volumeBackupMountDockerSock === 'boolean' ? res.volumeBackupMountDockerSock : true,
        notificationEnabledCategories: Array.isArray(res.notificationEnabledCategories)
          ? res.notificationEnabledCategories
          : [...defaultNotificationCategories]
      }
      editionFields.value = readEditionSettingsFields(res)
      
      if (res.allocPortStart) allocSettings.value.start = res.allocPortStart
      if (res.allocPortEnd) allocSettings.value.end = res.allocPortEnd
      if (typeof res.allowAutoAllocPort === 'boolean') {
        allocSettings.value.allowAutoAllocPort = res.allowAutoAllocPort
      }

      syncAdvancedModeLocal()
    }
  } catch (error) {
    console.error('加载设置失败:', error)
    toast.error('加载设置失败: ' + (error.message || '未知错误'))
  } finally {
    loading.value = false
  }
}

// 刷新


// 保存高级模式
const saveAdvancedMode = async (requestedValue) => {
  const next = typeof requestedValue === 'boolean'
    ? requestedValue
    : !settingsForm.value.advancedMode
  if (next === settingsForm.value.advancedMode) return
  const tip = next
    ? '开启后将允许修改并保存高风险的 YAML 配置入口，建议仅在明确知道修改内容时使用。是否继续？'
    : '关闭后将禁用高风险的 YAML 编辑与保存入口。是否继续？'
  
  const confirmed = await uiStore.confirm({
    type: next ? 'warning' : 'info',
    title: next ? '开启高级模式' : '关闭高级模式',
    message: tip,
    confirmText: next ? '开启' : '关闭'
  })
  if (!confirmed) return
  
  advancedModeLoading.value = true
  try {
    await settings.saveGlobal({ advancedMode: next })
    settingsForm.value.advancedMode = next
    syncAdvancedModeLocal()
    toast.success(next ? '高级模式已开启' : '高级模式已关闭')
  } catch (error) {
    console.error('保存失败:', error)
    toast.error('保存失败: ' + (error.message || '未知错误'))
  } finally {
    advancedModeLoading.value = false
  }
}

// 修改密码
const updatePassword = async () => {
  if (!passwordForm.value.oldPassword) {
    toast.warning('请输入当前密码')
    return
  }
  if (!passwordForm.value.newPassword) {
    toast.warning('请输入新密码')
    return
  }
  if (passwordForm.value.newPassword.length < 8) {
    toast.warning('新密码至少需要8个字符')
    return
  }
  if (passwordForm.value.newPassword !== passwordForm.value.confirmPassword) {
    toast.warning('两次输入的新密码不一致')
    return
  }
  
  passwordLoading.value = true
  try {
		const data = await auth.changePassword({
			oldPassword: passwordForm.value.oldPassword,
			newPassword: passwordForm.value.newPassword
		})
		if (data?.token) {
			localStorage.setItem('token', data.token)
		}
    toast.success('密码修改成功')
    passwordForm.value = { oldPassword: '', newPassword: '', confirmPassword: '' }
  } catch (error) {
    console.error('修改密码失败:', error)
    toast.error('修改密码失败: ' + (error.message || '未知错误'))
  } finally {
    passwordLoading.value = false
  }
}

// 保存服务配置
const saveServerSettings = async () => {
  serverLoading.value = true
  try {
    await settings.saveGlobal({
      lanUrl: settingsForm.value.lanUrl,
      wanUrl: settingsForm.value.wanUrl,
      appStoreCDNURL: settingsForm.value.appStoreCDNURL,
      ...getEditionServerFields(editionFields.value),
      advancedMode: settingsForm.value.advancedMode,
      imageUpdateIntervalMinutes: settingsForm.value.imageUpdateIntervalMinutes
    })
    syncAdvancedModeLocal()
    await loadCDNStatus()
    await fullServiceFieldsRef.value?.refresh?.()
    toast.success('配置已保存')
  } catch (error) {
    console.error('保存失败:', error)
    toast.error('保存失败: ' + (error.message || '未知错误'))
  } finally {
    serverLoading.value = false
  }
}

// 保存端口分配设置
const saveAllocSettings = async () => {
  if (!allocSettings.value.start || !allocSettings.value.end) {
    toast.warning('请输入端口范围')
    return
  }
  if (allocSettings.value.end <= allocSettings.value.start) {
    toast.warning('结束端口必须大于起始端口')
    return
  }
  
  allocLoading.value = true
  try {
    await settings.saveGlobal({
      allocPortStart: allocSettings.value.start,
      allocPortEnd: allocSettings.value.end,
      allowAutoAllocPort: allocSettings.value.allowAutoAllocPort
    })
    syncAdvancedModeLocal()
    toast.success('端口分配设置已保存')
  } catch (error) {
    console.error('保存失败:', error)
    toast.error('保存失败: ' + (error.message || '未知错误'))
  } finally {
    allocLoading.value = false
  }
}

// 刷新卷列表
const refreshVolumeOptions = async () => {
  try {
    const res = await volumes.list()
    const list = Array.isArray(res?.Volumes) ? res.Volumes : []
    volumeOptions.value = list.map((v) => v?.Name).filter(Boolean).sort()
  } catch (e) {
    volumeOptions.value = []
  }
}

function parseEnvLines(text) {
  const result = []
  const index = new Map()
  String(text || '').split('\n').forEach((line) => {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#') || !trimmed.includes('=')) {
      if (trimmed) result.push(line)
      return
    }
    const key = trimmed.split('=', 1)[0].trim()
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) {
      result.push(line)
      return
    }
    if (index.has(key)) {
      result[index.get(key)] = line
      return
    }
    index.set(key, result.length)
    result.push(line)
  })
  return { result, index }
}

function upsertEnvTemplate(current, template) {
  const { result, index } = parseEnvLines(current)
  template.forEach((line) => {
    const key = line.split('=', 1)[0].trim()
    if (index.has(key)) {
      result[index.get(key)] = line
      return
    }
    index.set(key, result.length)
    result.push(line)
  })
  return result.join('\n').trim()
}

function applyVolumeBackupStoragePreset() {
  const template = volumeBackupStorageTemplates[volumeBackupStoragePreset.value] || volumeBackupStorageTemplates.s3
  settingsForm.value.volumeBackupEnv = upsertEnvTemplate(settingsForm.value.volumeBackupEnv, template)
  toast.info(volumeBackupStoragePreset.value === 'webdav' ? '已写入 WebDAV 模板' : '已写入 S3 模板')
}

// 保存卷备份设置
const saveVolumeBackupSettings = async () => {
  volumeBackupSaving.value = true
  try {
    const payload = {
      volumeBackupEnabled: settingsForm.value.volumeBackupEnabled,
      volumeBackupImage: settingsForm.value.volumeBackupImage,
      volumeBackupCronExpression: settingsForm.value.volumeBackupCronExpression,
      volumeBackupVolumes: settingsForm.value.volumeBackupVolumes,
      volumeBackupArchiveDir: settingsForm.value.volumeBackupArchiveDir,
      volumeBackupMountDockerSock: settingsForm.value.volumeBackupMountDockerSock
    }
    if (settingsForm.value.volumeBackupEnv !== '') {
      payload.volumeBackupEnv = settingsForm.value.volumeBackupEnv
    }
    await settings.saveGlobal(payload)
    settingsForm.value.volumeBackupEnv = ''
    const res = await settings.getGlobal()
    settingsForm.value.volumeBackupEnvSet = !!res?.volumeBackupEnvSet
    settingsForm.value.volumeBackupEnvStored = !!res?.volumeBackupEnvStored
    syncAdvancedModeLocal()
    toast.success('卷备份配置已保存')
  } catch (error) {
    console.error('保存失败:', error)
    toast.error('保存失败: ' + (error.message || '未知错误'))
  } finally {
    volumeBackupSaving.value = false
  }
}

const saveNotificationSettings = async () => {
  notificationSaving.value = true
  try {
    await settings.saveGlobal({
      notificationEnabledCategories: settingsForm.value.notificationEnabledCategories
    })
    toast.success('通知偏好已保存')
  } catch (error) {
    console.error('保存失败:', error)
    toast.error('保存失败: ' + (error.message || '未知错误'))
  } finally {
    notificationSaving.value = false
  }
}

const notificationChannelTypeLabel = (type) => notificationChannelTypeOptions[type] || type || '未知类型'

const notificationCategorySummary = (categories) => {
  const labels = (Array.isArray(categories) ? categories : [])
    .map((category) => notificationCategoryOptions.find((item) => item.value === category)?.label)
    .filter(Boolean)
  return labels.length ? labels.join('、') : '全部分类'
}

const loadNotificationChannels = async () => {
  try {
    const result = await system.getNotificationChannels()
    notificationChannels.value = Array.isArray(result) ? result : []
  } catch (error) {
    console.error('加载外部通知通道失败:', error)
    toast.error('加载外部通知通道失败: ' + (error.message || '未知错误'))
  }
}

const openNotificationChannelDialog = (channel = null) => {
  editingNotificationChannel.value = channel
  notificationChannelForm.value = createNotificationChannelForm(channel)
  notificationChannelDialogVisible.value = true
}

const buildNotificationChannelPayload = () => {
  const form = notificationChannelForm.value
  const config = {}
  const secrets = {}
  if (form.type === 'webhook') {
    config.url = form.url.trim()
    if (form.signingSecret.trim()) secrets.signing_secret = form.signingSecret.trim()
  } else if (form.type === 'wecom') {
    config.server = form.server.trim()
    if (form.wecomKey.trim()) secrets.key = form.wecomKey.trim()
  } else if (form.type === 'ntfy') {
    config.server = form.server.trim() || 'https://ntfy.sh'
    config.topic = form.topic.trim()
    if (form.token.trim()) secrets.token = form.token.trim()
  } else if (form.type === 'gotify') {
    config.server = form.server.trim()
    if (form.token.trim()) secrets.token = form.token.trim()
  } else if (form.type === 'bark') {
    config.server = form.server.trim() || 'https://api.day.app'
    if (form.deviceKey.trim()) secrets.device_key = form.deviceKey.trim()
  } else if (form.type === 'pushplus') {
    Object.assign(config, buildPushPlusConfig(form))
    if (form.token.trim()) secrets.token = form.token.trim()
  }
  const payload = {
    name: form.name.trim(),
    type: form.type,
    enabled: form.enabled,
    categories: [...form.categories],
    config,
    clearSecrets: form.clearSecrets || (editingNotificationChannel.value && form.originalType !== form.type)
  }
  if (Object.keys(secrets).length) payload.secrets = secrets
  return payload
}

const saveNotificationChannel = async () => {
  notificationChannelSaving.value = true
  try {
    const payload = buildNotificationChannelPayload()
    if (editingNotificationChannel.value) {
      await system.updateNotificationChannel(editingNotificationChannel.value.id, payload)
    } else {
      await system.createNotificationChannel(payload)
    }
    notificationChannelDialogVisible.value = false
    await loadNotificationChannels()
    toast.success('通知通道已保存')
  } catch (error) {
    toast.error('保存通知通道失败: ' + (error.message || '未知错误'))
  } finally {
    notificationChannelSaving.value = false
  }
}

const testNotificationChannel = async (channel) => {
  notificationChannelTesting.value = channel.id
  try {
    await system.testNotificationChannel(channel.id)
    toast.success('测试通知已发送')
  } catch (error) {
    toast.error('测试通知失败: ' + (error.message || '未知错误'))
  } finally {
    notificationChannelTesting.value = ''
  }
}

const notificationDeliveryStatusLabel = (status) => ({
  succeeded: '已送达',
  failed: '发送失败',
  pending: '发送中'
}[status] || status || '未知状态')

const openNotificationDeliveryDialog = async (channel) => {
  notificationDeliveryChannel.value = channel
  notificationDeliveries.value = []
  notificationDeliveryDialogVisible.value = true
  try {
    const result = await system.getNotificationDeliveries(channel.id, { limit: 50 })
    notificationDeliveries.value = Array.isArray(result) ? result : []
  } catch (error) {
    toast.error('读取投递记录失败: ' + (error.message || '未知错误'))
  }
}

const removeNotificationChannel = async (channel) => {
  const confirmed = await uiStore.confirm({
    type: 'warning',
    title: '删除通知通道',
    message: `确定删除“${channel.name}”吗？历史投递记录会保留。`,
    confirmText: '删除'
  })
  if (!confirmed) return
  try {
    await system.deleteNotificationChannel(channel.id)
    await loadNotificationChannels()
    toast.success('通知通道已删除')
  } catch (error) {
    toast.error('删除通知通道失败: ' + (error.message || '未知错误'))
  }
}

// 重建卷备份容器
const rebuildVolumeBackup = async () => {
  if (!settingsForm.value.volumeBackupEnabled) {
    toast.warning('请先启用卷备份')
    return
  }
  const confirmed = await uiStore.confirm({
    type: 'warning',
    title: '重建卷备份容器',
    message: '确定要重建卷备份容器吗？',
    confirmText: '重建'
  })
  if (!confirmed) return
  
  volumeBackupRebuilding.value = true
  try {
    await system.rebuildVolumeBackup()
    toast.success('已触发重建')
  } catch (error) {
    console.error('重建失败:', error)
    toast.error('重建失败: ' + (error.message || '未知错误'))
  } finally {
    volumeBackupRebuilding.value = false
  }
}

// 保存 AI 设置
const saveAiSettings = async () => {
  aiSaving.value = true
  try {
    const payload = {
      aiEnabled: settingsForm.value.aiEnabled,
      ...getEditionAiFields(editionFields.value),
      aiBaseUrl: settingsForm.value.aiBaseUrl,
      aiModel: settingsForm.value.aiModel,
      aiUtilityModel: settingsForm.value.aiUtilityModel,
      aiTemperature: settingsForm.value.aiTemperature,
      aiMaxTokens: settingsForm.value.aiMaxTokens,
      aiAllowCreateCategory: settingsForm.value.aiAllowCreateCategory,
      aiNavigationPrompt: settingsForm.value.aiNavigationPrompt,
      aiComposePrompt: settingsForm.value.aiComposePrompt
    }
    if (settingsForm.value.aiApiKey !== '') {
      payload.aiApiKey = settingsForm.value.aiApiKey
    }
    await settings.saveGlobal(payload)
    settingsForm.value.aiApiKey = ''
    const res = await settings.getGlobal()
    settingsForm.value.aiApiKeySet = !!res?.aiApiKeySet
    settingsForm.value.aiApiKeyStored = !!res?.aiApiKeyStored
    settingsForm.value.aiApiKeySource = res?.aiApiKeySource || ''
    if (typeof res?.advancedMode === 'boolean') settingsForm.value.advancedMode = res.advancedMode
    syncAdvancedModeLocal()
    toast.success('AI 配置已保存')
  } catch (error) {
    console.error('保存失败:', error)
    toast.error('保存失败: ' + (error.message || '未知错误'))
  } finally {
    aiSaving.value = false
  }
}

// 模型字段从下拉选中（含自定义输入确认）后立即持久化，载荷与「保存 AI 配置」一致
const autoSaveAiSettings = () => {
  saveAiSettings()
}

// 清空 AI API Key
const clearAiApiKey = async () => {
  aiClearing.value = true
  try {
    await settings.saveGlobal({
      aiEnabled: settingsForm.value.aiEnabled,
      ...getEditionAiFields(editionFields.value),
      aiBaseUrl: settingsForm.value.aiBaseUrl,
      aiModel: settingsForm.value.aiModel,
      aiUtilityModel: settingsForm.value.aiUtilityModel,
      aiTemperature: settingsForm.value.aiTemperature,
      aiMaxTokens: settingsForm.value.aiMaxTokens,
      aiAllowCreateCategory: settingsForm.value.aiAllowCreateCategory,
      aiNavigationPrompt: settingsForm.value.aiNavigationPrompt,
      aiComposePrompt: settingsForm.value.aiComposePrompt,
      aiApiKey: ''
    })
    settingsForm.value.aiApiKey = ''
    const res = await settings.getGlobal()
    settingsForm.value.aiApiKeySet = !!res?.aiApiKeySet
    settingsForm.value.aiApiKeyStored = !!res?.aiApiKeyStored
    settingsForm.value.aiApiKeySource = res?.aiApiKeySource || ''
    syncAdvancedModeLocal()
    toast.success('Key 已清空')
  } catch (error) {
    console.error('清空失败:', error)
    toast.error('清空失败: ' + (error.message || '未知错误'))
  } finally {
    aiClearing.value = false
  }
}

// 拉取 AI 提供商模型列表：使用表单当前 Base URL，API Key 留空时由后端使用已保存的 Key
const fetchAiModels = async () => {
  if (!ensureAISettingsReady()) return
  aiModelsLoading.value = true
  aiModelsError.value = ''
  try {
    const payload = { baseUrl: settingsForm.value.aiBaseUrl }
    if (settingsForm.value.aiApiKey !== '') {
      payload.apiKey = settingsForm.value.aiApiKey
    }
    const res = await aiApi.listModels(payload)
    // 拉取成功才写缓存；缓存同时证明 Base URL + Key 连通
    aiModelsCache.value = writeAiModelsCache({
      baseUrl: settingsForm.value.aiBaseUrl,
      models: res?.models || []
    })
    toast.success(`已获取 ${aiModelsCache.value.models.length} 个模型`)
  } catch (error) {
    // 失败时保留旧缓存，只展示错误
    console.error('获取模型列表失败:', error)
    aiModelsError.value = '获取模型列表失败: ' + (error.message || '未知错误')
    toast.error(aiModelsError.value)
  } finally {
    aiModelsLoading.value = false
  }
}

// 测试 AI 连接
const testAiConnectivity = async () => {
  if (!ensureAISettingsReady()) return
  aiTesting.value = true
  aiTestResult.value = null
  try {
    const payload = {
      enabled: true,
      baseUrl: settingsForm.value.aiBaseUrl,
      model: settingsForm.value.aiUtilityModel || settingsForm.value.aiModel,
      temperature: settingsForm.value.aiTemperature
    }
    if (settingsForm.value.aiApiKey !== '') {
      payload.apiKey = settingsForm.value.aiApiKey
    }
    const res = await testAI(payload)
    aiTestResult.value = formatAIDiagnosticResult(res)
    if (aiTestResult.value.ok) {
      toast.success(aiTestResult.value.message)
    } else {
      toast.warning(aiTestResult.value.message)
    }
  } catch (error) {
    console.error('连接失败:', error)
    aiTestResult.value = {
      ok: false,
      message: '连接失败',
      detail: error.message || '未知错误'
    }
    toast.error('连接失败: ' + (error.message || '未知错误'))
  } finally {
    aiTesting.value = false
  }
}

function diagnosticFilename(now = new Date()) {
  const pad = value => String(value).padStart(2, '0')
  return `tradis-diagnostics-${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}.zip`
}

async function exportDiagnosticBundle() {
  diagnosticExporting.value = true
  try {
    const blob = await system.exportDiagnostics()
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = diagnosticFilename()
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)
    toast.success('诊断包已导出')
  } catch (error) {
    console.error('导出诊断包失败:', error)
    toast.error('导出诊断包失败: ' + (error.message || '未知错误'))
  } finally {
    diagnosticExporting.value = false
  }
}

async function refreshSettingsPage() {
  await Promise.all([
    loadSettings(),
    loadDeploymentPolicy(),
    refreshVolumeOptions(),
    loadNotificationChannels()
  ])
  if (isFullEdition) {
    await Promise.all([loadCDNStatus(), fullServiceFieldsRef.value?.refresh?.(), loadLicenseStatus()])
  }
  toast.success('设置已刷新')
}

onMounted(async () => {
  settingsMounted.value = true
  themeStore.init()
  try {
    await loadFullSettingsDependencies()
  } catch (error) {
    console.error('加载版本专属设置依赖失败:', error)
  }
  loadSettings()
  loadDeploymentPolicy()
  refreshVolumeOptions()
  loadNotificationChannels()
  if (isFullEdition) {
    loadCDNStatus()
    fullServiceFieldsRef.value?.refresh?.()
    loadLicenseStatus()
  }
  if (route.query?.section) await scrollSettingSectionIntoView(route.query.section)
})
</script>

<style scoped>
.settings-workbench {
  --resource-toolbar-search-width: 360px;
  --resource-accent: var(--color-primary-500);
}

.settings-page {
  box-sizing: border-box;
  width: 100%;
  height: 100%;
  min-height: 0;
  padding: 0 2px 24px;
  overflow-y: auto;
  scrollbar-gutter: stable;
}

.toolbar-right,
.secondary-btn,
.settings-category-tabs,
.filter-tab {
  display: flex;
  align-items: center;
}

.toolbar-right {
  flex: 0 0 auto;
}

.secondary-btn {
  justify-content: center;
  gap: 7px;
  min-height: 38px;
  padding: 0 14px;
  border: 1px solid var(--border-default);
  border-radius: 9px;
  background: var(--bg-elevated);
  color: var(--text-primary);
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), border-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out);
}

.secondary-btn:hover:not(:disabled) {
  border-color: var(--color-primary-300);
  background: var(--color-primary-100);
  color: var(--color-primary-700);
}

.secondary-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.settings-category-tabs {
  flex: 1 1 auto;
  min-width: 0;
  gap: 4px;
  padding: 3px;
  overflow-x: auto;
  border: 1px solid var(--border-subtle);
  border-radius: 9px;
  background: var(--bg-secondary);
  scrollbar-width: none;
}

.settings-category-tabs::-webkit-scrollbar {
  display: none;
}

.filter-tab {
  justify-content: center;
  min-height: 30px;
  padding: 0 12px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: var(--text-secondary);
  font-size: 0.8125rem;
  font-weight: 500;
  white-space: nowrap;
  cursor: pointer;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out), color var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.filter-tab:hover {
  color: var(--text-primary);
  background: var(--bg-elevated);
}

.filter-tab.active {
  color: var(--color-primary-700);
  background: var(--color-primary-100);
  box-shadow: inset 0 0 0 1px var(--color-primary-200);
}

.settings-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 280px;
  width: min(100%, 960px);
  margin-inline: auto;
  color: var(--text-tertiary);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  background: var(--resource-panel, var(--bg-primary));
}

.settings-empty strong {
  color: var(--text-primary);
  font-size: 0.9375rem;
}

.settings-empty span {
  font-size: 0.8125rem;
}

/* 设置双栏流 */
.settings-columns {
  width: 100%;
  column-count: 2;
  column-gap: 16px;
}

.settings-column-start {
  break-before: column;
}

/* 设置卡片 */
.settings-card {
  display: block;
  width: 100%;
  margin-bottom: 16px;
  break-inside: avoid-column;
  background: var(--bg-elevated);
  border: 1px solid var(--border-subtle);
  border-radius: 12px;
  overflow: hidden;
  scroll-margin-top: 8px;
  transition: border-color var(--motion-duration-quick) var(--motion-ease-out), box-shadow var(--motion-duration-quick) var(--motion-ease-out);
}

.settings-card:hover {
  border-color: var(--border-default);
  box-shadow: var(--shadow-md);
}

.cdn-status {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-top: 10px;
  padding: 10px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  color: var(--text-secondary);
  background: var(--bg-secondary);
  font-size: 0.8125rem;
}

.cdn-status.is-success {
  color: var(--color-success-600);
  border-color: color-mix(in srgb, var(--color-success-500) 32%, var(--border-subtle));
}

.cdn-status.is-info {
  color: var(--color-primary);
  border-color: color-mix(in srgb, var(--color-primary) 32%, var(--border-subtle));
}

.cdn-status.is-warning {
  color: var(--color-warning-700);
  border-color: color-mix(in srgb, var(--color-warning-500) 32%, var(--border-subtle));
}

.cdn-status-content {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.cdn-status-time {
  color: var(--text-tertiary);
  font-size: 12px;
}

.spin-icon {
  animation: spin 0.8s linear infinite;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 52px;
  padding: 12px 18px;
  border-bottom: 1px solid var(--border-subtle);
  background: color-mix(in srgb, var(--bg-secondary) 72%, var(--bg-elevated));
}

.card-title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 0.9375rem;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0;
}

.card-icon {
  color: var(--color-primary-500);
  flex-shrink: 0;
}

.card-body {
  padding: 18px;
}

.setting-disclosure {
  overflow: hidden;
  border: 1px solid var(--border-subtle);
  border-radius: 9px;
  background: var(--bg-secondary);
}

.setting-disclosure + .setting-disclosure,
.setting-disclosure + .form-actions {
  margin-top: 12px;
}

.setting-disclosure__summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 48px;
  padding: 9px 12px;
  color: var(--text-primary);
  cursor: pointer;
  list-style: none;
  transition: background-color var(--motion-duration-quick) var(--motion-ease-out);
}

.setting-disclosure__summary::-webkit-details-marker {
  display: none;
}

.setting-disclosure__summary:hover {
  background: var(--bg-tertiary);
}

.setting-disclosure__summary > span {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.setting-disclosure__summary strong {
  font-size: 0.8125rem;
  font-weight: 600;
}

.setting-disclosure__summary small {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  font-weight: 400;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.setting-disclosure__summary > svg {
  flex: 0 0 auto;
  color: var(--text-tertiary);
  transition: transform var(--motion-duration-quick) var(--motion-ease-out);
}

.setting-disclosure[open] .setting-disclosure__summary > svg {
  transform: rotate(180deg);
}

.setting-disclosure__body {
  padding: 14px 12px 12px;
  border-top: 1px solid var(--border-subtle);
  background: var(--bg-elevated);
}

.profile-identity-grid,
.nas-library-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.nas-library-grid {
  margin-top: 16px;
}

.profile-textarea {
  min-height: 58px;
  font-family: var(--font-sans, Inter, ui-sans-serif, system-ui, sans-serif);
}

.notification-channels {
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid var(--border-subtle);
}

.notification-channels-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.notification-channels-header .form-label {
  margin-bottom: 0;
}

.notification-add-btn {
  flex: 0 0 auto;
  height: 32px;
  padding: 0 10px;
  gap: 6px;
  font-size: 0.8125rem;
}

.notification-channel-list {
  display: grid;
  gap: 8px;
  margin-top: 12px;
}

.notification-channel-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-width: 0;
  gap: 12px;
  padding: 10px 11px;
  border: 1px solid var(--border-subtle);
  border-radius: 7px;
  background: color-mix(in srgb, var(--bg-secondary) 82%, transparent);
}

.notification-channel-main {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.notification-channel-main strong {
  overflow: hidden;
  color: var(--text-primary);
  font-size: 0.8125rem;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notification-channel-main span {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notification-channel-actions {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 5px;
}

.notification-channel-state {
  color: var(--color-success-600);
  font-size: 0.75rem;
  white-space: nowrap;
}

.notification-channel-state.muted {
  color: var(--text-tertiary);
}

.notification-channel-empty {
  margin-bottom: 0;
  padding: 10px 0 0;
}

.notification-delivery-list {
  display: grid;
  gap: 8px;
}

.notification-delivery-row {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 10px 0;
  border-bottom: 1px solid var(--border-subtle);
}

.notification-delivery-row:last-child {
  border-bottom: 0;
}

.notification-delivery-row > div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.notification-delivery-row strong {
  color: var(--text-primary);
  font-size: 0.8125rem;
}

.notification-delivery-row span,
.notification-delivery-row small,
.notification-delivery-row time {
  color: var(--text-tertiary);
  font-size: 0.75rem;
}

.notification-delivery-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notification-delivery-row time {
  flex: 0 0 auto;
  white-space: nowrap;
}

/* 表单样式 */
.form-group {
  margin-bottom: 16px;
}

.form-group:last-child {
  margin-bottom: 0;
}

.form-label {
  display: block;
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
  margin-bottom: 6px;
}

.form-input,
.form-textarea {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-size: 0.875rem;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.form-input:focus,
.form-textarea:focus {
  outline: none;
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.form-textarea {
  resize: vertical;
  min-height: 80px;
  font-family: var(--font-mono, 'JetBrains Mono', monospace);
}

.inline-field {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}

.inline-field .form-input {
  flex: 1 1 220px;
  width: auto;
}

.form-hint {
  font-size: 0.75rem;
  color: var(--text-tertiary);
  margin: 6px 0 0;
}

.ai-preset-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 8px;
}

.ai-model-fetch {
  display: flex;
  align-items: center;
  gap: 12px;
}

.ai-models-error {
  color: var(--color-danger-600);
}

.ai-preset-card {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  width: 100%;
  padding: 12px;
  border: 1px solid var(--border-subtle);
  border-radius: 10px;
  background: var(--bg-secondary);
  text-align: left;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.ai-preset-card:hover {
  border-color: var(--color-primary-500);
  box-shadow: 0 0 0 3px var(--ring-primary);
}

.ai-preset-name {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
}

.ai-preset-endpoint {
  font-size: 0.6875rem;
  color: var(--color-primary-600);
}

.ai-preset-model {
  font-size: 0.75rem;
  color: var(--text-tertiary);
}

.checkbox-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 0.875rem;
  color: var(--text-primary);
  cursor: pointer;
}

.checkbox-label input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

.setting-hint {
  margin: 6px 0 0;
  color: var(--text-tertiary);
  font-size: 0.75rem;
  line-height: 1.5;
}

.checkbox-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 14px;
}

/* 主题色选择器 */
.color-theme-selector {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.color-theme-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border: 2px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.color-theme-btn:hover {
  border-color: var(--theme-color);
}

.color-theme-btn.active {
  border-color: var(--theme-color);
  background: var(--bg-elevated);
}

.color-dot {
  width: 16px;
  height: 16px;
  border-radius: 50%;
  border: 2px solid var(--border-subtle);
}

.color-name {
  font-size: 0.875rem;
  color: var(--text-primary);
}

/* 图标集选择器 */
.icon-set-selector {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.icon-set-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 14px;
  border: 2px solid var(--border-subtle);
  border-radius: 8px;
  background: var(--bg-secondary);
  color: var(--text-secondary);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: all var(--motion-duration-quick) var(--motion-ease-out);
}

.icon-set-btn:hover {
  border-color: var(--color-primary-400);
}

.icon-set-btn.active {
  border-color: var(--color-primary-500);
  background: var(--color-primary-500-10);
}

.icon-preview {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--color-primary-500);
}

.icon-set-name {
  font-size: 0.875rem;
  color: var(--text-primary);
}

/* 范围输入 */
.range-inputs {
  display: flex;
  align-items: center;
  gap: 12px;
}

.range-inputs .form-input {
  flex: 1;
}

.range-separator {
  color: var(--text-secondary);
  font-weight: 500;
}

/* 警告提示 */
.alert {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  border-radius: 8px;
  font-size: 0.8125rem;
  margin-top: 8px;
}

.alert-warning {
  background: var(--color-warning-100);
  border: 1px solid var(--color-warning-200);
  color: var(--color-warning-700);
}

.alert-success {
  background: var(--color-success-100);
  border: 1px solid var(--color-success-200);
  color: var(--color-success-700);
}

.alert-danger {
  background: var(--color-danger-100);
  border: 1px solid var(--color-danger-200);
  color: var(--color-danger-700);
}

.alert-detail {
  margin-top: 4px;
  font-size: 0.75rem;
  color: var(--text-secondary);
  word-break: break-all;
}

.alert svg {
  stroke: currentColor;
  fill: none;
  stroke-width: 2;
  stroke-linecap: round;
  stroke-linejoin: round;
  flex-shrink: 0;
}

/* 按钮组 */
.form-actions {
  display: flex;
  gap: 10px;
  margin-top: 12px;
  flex-wrap: wrap;
}

.btn {
  position: relative;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 38px;
  padding: 0 16px;
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  cursor: pointer;
  transition:
    transform var(--motion-duration-quick) var(--motion-ease-out),
    box-shadow var(--motion-duration-quick) var(--motion-ease-out),
    background-color var(--motion-duration-quick) var(--motion-ease-out),
    border-color var(--motion-duration-quick) var(--motion-ease-out),
    opacity var(--motion-duration-quick) var(--motion-ease-out);
  border: none;
}

.btn-primary {
  background: linear-gradient(135deg, var(--color-primary-500), var(--color-primary-600));
  color: var(--text-inverse);
}

.btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px var(--color-primary-500-30);
}

.btn-default {
  background: var(--bg-tertiary);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
}

.btn-default:hover:not(:disabled) {
  background: var(--border-default);
}

.btn-danger {
  background: var(--color-danger-100);
  color: var(--color-danger-600);
  border: 1px solid var(--color-danger-200);
}

.btn-danger:hover:not(:disabled) {
  background: var(--color-danger-200);
}

.btn-warning {
  background: var(--color-warning-100);
  color: var(--color-warning-700);
  border: 1px solid var(--color-warning-200);
}

.btn-warning:hover:not(:disabled) {
  background: var(--color-warning-200);
}

.btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.settings-page button:not(:disabled):active {
  transform: translateY(1px) scale(0.97);
}

.btn.is-loading {
  cursor: wait;
  opacity: 0.88;
  transform: none;
  box-shadow: inset 0 0 0 1px color-mix(in srgb, currentColor 12%, transparent);
}

.btn.is-loading::before {
  content: '';
  position: absolute;
  right: 9px;
  top: 50%;
  z-index: 2;
  width: 14px;
  height: 14px;
  margin-top: -7px;
  border: 2px solid color-mix(in srgb, var(--text-inverse) 32%, transparent);
  border-top-color: var(--text-inverse);
  border-radius: 50%;
  animation: resource-refresh-spin 0.7s linear infinite;
  pointer-events: none;
}

.btn-spinner {
  display: none;
}

.btn-default.is-loading::before,
.btn-danger.is-loading::before,
.btn-warning.is-loading::before {
  border-color: color-mix(in srgb, var(--text-primary) 24%, transparent);
  border-top-color: var(--text-primary);
}

/* 间距工具 */
.mt-2 {
  margin-top: 8px;
}

/* 响应式 */
@media (max-width: 1200px) {
  .settings-category-tabs {
    flex: 1 1 220px;
  }
}

@media (max-width: 900px) {
  .settings-columns {
    column-count: 1;
  }

  .settings-column-start {
    break-before: auto;
  }
}

@media (max-width: 640px) {
  .settings-page {
    padding: 0 0 16px;
  }

  .settings-workbench {
    --resource-toolbar-search-width: 100%;
  }

  .toolbar-right,
  .secondary-btn {
    width: 100%;
  }
  
  .range-inputs {
    flex-direction: column;
    align-items: stretch;
  }
  
  .range-separator {
    text-align: center;
  }
  
  .form-actions {
    flex-direction: column;
  }

  .notification-channels-header,
  .notification-channel-row {
    align-items: stretch;
    flex-direction: column;
  }

  .notification-channel-actions {
    justify-content: flex-end;
  }

  .notification-delivery-row {
    align-items: flex-start;
    flex-direction: column;
    gap: 6px;
  }

  .ai-preset-list {
    grid-template-columns: 1fr;
  }

  .profile-identity-grid,
  .nas-library-grid {
    grid-template-columns: 1fr;
  }

  .btn {
    width: 100%;
  }
}
</style>
