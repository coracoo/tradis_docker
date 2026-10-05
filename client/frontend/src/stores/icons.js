import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

// 图标映射表 - 将通用图标名称映射到不同图标库的具体图标
const iconMapping = {
  // 通用图标名称: { lucide: 'lucide:图标名', tabler: 'tabler:图标名' }
  'box': { lucide: 'lucide:container', tabler: 'tabler:box' },
  'folder': { lucide: 'lucide:folder', tabler: 'tabler:folder' },
  'image': { lucide: 'lucide:image', tabler: 'tabler:photo' },
  'network': { lucide: 'lucide:network', tabler: 'tabler:network' },
  'database': { lucide: 'lucide:database', tabler: 'tabler:database' },
  'settings': { lucide: 'lucide:settings', tabler: 'tabler:settings' },
  'dashboard': { lucide: 'lucide:layout-dashboard', tabler: 'tabler:layout-dashboard' },
  'containers': { lucide: 'lucide:containers', tabler: 'tabler:container' },
  'compose': { lucide: 'lucide:file-stack', tabler: 'tabler:stack' },
  'terminal': { lucide: 'lucide:terminal', tabler: 'tabler:terminal' },
  'logs': { lucide: 'lucide:scroll-text', tabler: 'tabler:file-text' },
  'start': { lucide: 'lucide:play', tabler: 'tabler:player-play' },
  'stop': { lucide: 'lucide:square', tabler: 'tabler:square' },
  'restart': { lucide: 'lucide:rotate-cw', tabler: 'tabler:refresh' },
  'delete': { lucide: 'lucide:trash-2', tabler: 'tabler:trash' },
  'edit': { lucide: 'lucide:edit-3', tabler: 'tabler:edit' },
  'search': { lucide: 'lucide:search', tabler: 'tabler:search' },
  'plus': { lucide: 'lucide:plus', tabler: 'tabler:plus' },
  'minus': { lucide: 'lucide:minus', tabler: 'tabler:minus' },
  'close': { lucide: 'lucide:x', tabler: 'tabler:x' },
  'check': { lucide: 'lucide:check', tabler: 'tabler:check' },
  'chevron-down': { lucide: 'lucide:chevron-down', tabler: 'tabler:chevron-down' },
  'chevron-up': { lucide: 'lucide:chevron-up', tabler: 'tabler:chevron-up' },
  'more': { lucide: 'lucide:more-vertical', tabler: 'tabler:dots-vertical' },
  'refresh': { lucide: 'lucide:refresh-cw', tabler: 'tabler:refresh' },
  'download': { lucide: 'lucide:download', tabler: 'tabler:download' },
  'upload': { lucide: 'lucide:upload', tabler: 'tabler:upload' },
  'copy': { lucide: 'lucide:copy', tabler: 'tabler:copy' },
  'eye': { lucide: 'lucide:eye', tabler: 'tabler:eye' },
  'eye-off': { lucide: 'lucide:eye-off', tabler: 'tabler:eye-off' },
  'sun': { lucide: 'lucide:sun', tabler: 'tabler:sun' },
  'moon': { lucide: 'lucide:moon', tabler: 'tabler:moon' },
  'bell': { lucide: 'lucide:bell', tabler: 'tabler:bell' },
  'user': { lucide: 'lucide:user', tabler: 'tabler:user' },
  'lock': { lucide: 'lucide:lock', tabler: 'tabler:lock' },
  'unlock': { lucide: 'lucide:unlock', tabler: 'tabler:lock-open' },
  'cpu': { lucide: 'lucide:cpu', tabler: 'tabler:cpu' },
  'memory': { lucide: 'lucide:hard-drive', tabler: 'tabler:device-analytics' },
  'disk': { lucide: 'lucide:database', tabler: 'tabler:database' },
  'grid': { lucide: 'lucide:layout-grid', tabler: 'tabler:layout-grid' },
  'table': { lucide: 'lucide:table', tabler: 'tabler:table' },
  'list': { lucide: 'lucide:list', tabler: 'tabler:list' },
  'filter': { lucide: 'lucide:filter', tabler: 'tabler:filter' },
  'sort': { lucide: 'lucide:arrow-up-down', tabler: 'tabler:arrows-sort' },
  'tag': { lucide: 'lucide:tag', tabler: 'tabler:tag' },
  'info': { lucide: 'lucide:info', tabler: 'tabler:info-circle' },
  'warning': { lucide: 'lucide:alert-triangle', tabler: 'tabler:alert-triangle' },
  'error': { lucide: 'lucide:alert-circle', tabler: 'tabler:alert-circle' },
  'success': { lucide: 'lucide:check-circle', tabler: 'tabler:circle-check' },
  'clock': { lucide: 'lucide:clock', tabler: 'tabler:clock' },
  'calendar': { lucide: 'lucide:calendar', tabler: 'tabler:calendar' },
  'home': { lucide: 'lucide:home', tabler: 'tabler:home' },
  'arrow-left': { lucide: 'lucide:arrow-left', tabler: 'tabler:arrow-left' },
  'arrow-right': { lucide: 'lucide:arrow-right', tabler: 'tabler:arrow-right' },
  'arrow-up': { lucide: 'lucide:arrow-up', tabler: 'tabler:arrow-up' },
  'arrow-down': { lucide: 'lucide:arrow-down', tabler: 'tabler:arrow-down' },
  'external-link': { lucide: 'lucide:external-link', tabler: 'tabler:external-link' },
  'link': { lucide: 'lucide:link', tabler: 'tabler:link' },
  'unlink': { lucide: 'lucide:unlink', tabler: 'tabler:link-off' },
  'menu': { lucide: 'lucide:menu', tabler: 'tabler:menu' },
  'apps': { lucide: 'lucide:layout-grid', tabler: 'tabler:apps' },
  'store': { lucide: 'lucide:store', tabler: 'tabler:store' },
  'chart': { lucide: 'lucide:bar-chart-3', tabler: 'tabler:chart-bar' },
  'activity': { lucide: 'lucide:activity', tabler: 'tabler:activity' },
  'zap': { lucide: 'lucide:zap', tabler: 'tabler:bolt' },
  'shield': { lucide: 'lucide:shield', tabler: 'tabler:shield' },
  'key': { lucide: 'lucide:key', tabler: 'tabler:key' },
  'mail': { lucide: 'lucide:mail', tabler: 'tabler:mail' },
  'phone': { lucide: 'lucide:phone', tabler: 'tabler:phone' },
  'map': { lucide: 'lucide:map', tabler: 'tabler:map' },
  'globe': { lucide: 'lucide:globe', tabler: 'tabler:globe' },
  'wifi': { lucide: 'lucide:wifi', tabler: 'tabler:wifi' },
  'bluetooth': { lucide: 'lucide:bluetooth', tabler: 'tabler:bluetooth' },
  'battery': { lucide: 'lucide:battery', tabler: 'tabler:battery' },
  'power': { lucide: 'lucide:power', tabler: 'tabler:power' },
  'volume': { lucide: 'lucide:volume-2', tabler: 'tabler:volume' },
  'volume-off': { lucide: 'lucide:volume-x', tabler: 'tabler:volume-off' },
  'maximize': { lucide: 'lucide:maximize', tabler: 'tabler:maximize' },
  'minimize': { lucide: 'lucide:minimize', tabler: 'tabler:minimize' },
  'fullscreen': { lucide: 'lucide:maximize-2', tabler: 'tabler:maximize' },
  'exit-fullscreen': { lucide: 'lucide:minimize-2', tabler: 'tabler:minimize' },
  'sidebar': { lucide: 'lucide:panel-left', tabler: 'tabler:layout-sidebar' },
  'sidebar-right': { lucide: 'lucide:panel-right', tabler: 'tabler:layout-sidebar-right' },
  'panel-top': { lucide: 'lucide:panel-top', tabler: 'tabler:layout-navbar' },
  'panel-bottom': { lucide: 'lucide:panel-bottom', tabler: 'tabler:layout-navbar-inactive' },
  'layers': { lucide: 'lucide:layers', tabler: 'tabler:layers' },
  'stack': { lucide: 'lucide:layers', tabler: 'tabler:stack' },
  'cube': { lucide: 'lucide:box', tabler: 'tabler:cube' },
  'cubes': { lucide: 'lucide:boxes', tabler: 'tabler:cubes' },
  'server': { lucide: 'lucide:server', tabler: 'tabler:server' },
  'cloud': { lucide: 'lucide:cloud', tabler: 'tabler:cloud' },
  'cloud-upload': { lucide: 'lucide:cloud-upload', tabler: 'tabler:cloud-upload' },
  'cloud-download': { lucide: 'lucide:cloud-download', tabler: 'tabler:cloud-download' },
  'cloud-off': { lucide: 'lucide:cloud-off', tabler: 'tabler:cloud-off' },
  'package': { lucide: 'lucide:package', tabler: 'tabler:package' },
  'layer': { lucide: 'lucide:layers', tabler: 'tabler:layers' },
  'backup': { lucide: 'lucide:archive-restore', tabler: 'tabler:archive' },
  'archive': { lucide: 'lucide:archive', tabler: 'tabler:archive' },
  'file': { lucide: 'lucide:file', tabler: 'tabler:file' },
  'file-text': { lucide: 'lucide:file-text', tabler: 'tabler:file-text' },
  'file-code': { lucide: 'lucide:file-code', tabler: 'tabler:file-code' },
  'folder-open': { lucide: 'lucide:folder-open', tabler: 'tabler:folder-open' },
  'folder-plus': { lucide: 'lucide:folder-plus', tabler: 'tabler:folder-plus' },
  'folder-minus': { lucide: 'lucide:folder-minus', tabler: 'tabler:folder-minus' },
  'git-branch': { lucide: 'lucide:git-branch', tabler: 'tabler:git-branch' },
  'git-commit': { lucide: 'lucide:git-commit', tabler: 'tabler:git-commit' },
  'git-merge': { lucide: 'lucide:git-merge', tabler: 'tabler:git-merge' },
  'git-pull-request': { lucide: 'lucide:git-pull-request', tabler: 'tabler:git-pull-request' },
  'github': { lucide: 'lucide:github', tabler: 'tabler:brand-github' },
  'gitlab': { lucide: 'lucide:gitlab', tabler: 'tabler:brand-gitlab' },
  'docker': { lucide: 'simple-icons:docker', tabler: 'simple-icons:docker' },
  'kubernetes': { lucide: 'lucide:hexagon', tabler: 'tabler:brand-kubernetes' },
  'aws': { lucide: 'lucide:cloud', tabler: 'tabler:brand-aws' },
  'azure': { lucide: 'lucide:cloud', tabler: 'tabler:brand-azure' },
  'gcp': { lucide: 'lucide:cloud', tabler: 'tabler:brand-google' },
  'terminal': { lucide: 'lucide:terminal', tabler: 'tabler:terminal' },
  'code': { lucide: 'lucide:code', tabler: 'tabler:code' },
  'code-2': { lucide: 'lucide:code-2', tabler: 'tabler:code' },
  'bug': { lucide: 'lucide:bug', tabler: 'tabler:bug' },
  'brush': { lucide: 'lucide:brush', tabler: 'tabler:brush' },
  'palette': { lucide: 'lucide:palette', tabler: 'tabler:palette' },
  'pen': { lucide: 'lucide:pen-tool', tabler: 'tabler:pen' },
  'pencil': { lucide: 'lucide:pencil', tabler: 'tabler:pencil' },
  'eraser': { lucide: 'lucide:eraser', tabler: 'tabler:eraser' },
  'scissors': { lucide: 'lucide:scissors', tabler: 'tabler:scissors' },
  'ruler': { lucide: 'lucide:ruler', tabler: 'tabler:ruler' },
  'compass': { lucide: 'lucide:compass', tabler: 'tabler:compass' },
  'map-pin': { lucide: 'lucide:map-pin', tabler: 'tabler:map-pin' },
  'navigation': { lucide: 'lucide:navigation', tabler: 'tabler:navigation' },
  'navigation-2': { lucide: 'lucide:navigation-2', tabler: 'tabler:navigation' },
  'locate': { lucide: 'lucide:locate', tabler: 'tabler:locate' },
  'locate-fixed': { lucide: 'lucide:locate-fixed', tabler: 'tabler:locate-fixed' },
  'aim': { lucide: 'lucide:aim', tabler: 'tabler:target' },
  'target': { lucide: 'lucide:target', tabler: 'tabler:target' },
  'crosshair': { lucide: 'lucide:crosshair', tabler: 'tabler:crosshair' },
  'send': { lucide: 'lucide:send', tabler: 'tabler:send' },
  'share': { lucide: 'lucide:share-2', tabler: 'tabler:share' },
  'share-2': { lucide: 'lucide:share', tabler: 'tabler:share-2' },
  'message': { lucide: 'lucide:message-square', tabler: 'tabler:message' },
  'message-circle': { lucide: 'lucide:message-circle', tabler: 'tabler:message-circle' },
  'chat': { lucide: 'lucide:message-square', tabler: 'tabler:messages' },
  'comment': { lucide: 'lucide:message-circle', tabler: 'tabler:message' },
  'heart': { lucide: 'lucide:heart', tabler: 'tabler:heart' },
  'star': { lucide: 'lucide:star', tabler: 'tabler:star' },
  'bookmark': { lucide: 'lucide:bookmark', tabler: 'tabler:bookmark' },
  'flag': { lucide: 'lucide:flag', tabler: 'tabler:flag' },
  'pin': { lucide: 'lucide:pin', tabler: 'tabler:pin' },
  'book': { lucide: 'lucide:book', tabler: 'tabler:book' },
  'book-open': { lucide: 'lucide:book-open', tabler: 'tabler:book-open' },
  'graduation-cap': { lucide: 'lucide:graduation-cap', tabler: 'tabler:school' },
  'award': { lucide: 'lucide:award', tabler: 'tabler:award' },
  'medal': { lucide: 'lucide:medal', tabler: 'tabler:medal' },
  'trophy': { lucide: 'lucide:trophy', tabler: 'tabler:trophy' },
  'gift': { lucide: 'lucide:gift', tabler: 'tabler:gift' },
  'crown': { lucide: 'lucide:crown', tabler: 'tabler:crown' },
  'gem': { lucide: 'lucide:gem', tabler: 'tabler:diamond' },
  'diamond': { lucide: 'lucide:diamond', tabler: 'tabler:diamond' },
  
  // 设置页面图标
  'appearance': { lucide: 'lucide:sun-moon', tabler: 'tabler:sun-moon' },
  'advanced': { lucide: 'lucide:sparkles', tabler: 'tabler:sparkles' },
  'security': { lucide: 'lucide:lock', tabler: 'tabler:lock' },
  'server-config': { lucide: 'lucide:server-cog', tabler: 'tabler:server-cog' },
  'ports': { lucide: 'lucide:network', tabler: 'tabler:network' },
  'ai': { lucide: 'lucide:bot', tabler: 'tabler:robot' },
  'volume-backup': { lucide: 'lucide:archive-restore', tabler: 'tabler:archive' },
  'warning': { lucide: 'lucide:alert-triangle', tabler: 'tabler:alert-triangle' },
  'rocket': { lucide: 'lucide:rocket', tabler: 'tabler:rocket' },
  
  // Compose 页面图标
  'container': { lucide: 'lucide:container', tabler: 'tabler:container' },
  'container-detail': { lucide: 'lucide:container', tabler: 'tabler:container' },
  'compose-file': { lucide: 'lucide:file-code', tabler: 'tabler:file-code' },
  'env-file': { lucide: 'lucide:file-key', tabler: 'tabler:file' },
  'template': { lucide: 'lucide:layout-template', tabler: 'tabler:template' },
  'ai-generate': { lucide: 'lucide:sparkles', tabler: 'tabler:sparkles' },
  'deploy-logs': { lucide: 'lucide:scroll-text', tabler: 'tabler:file-text' },
  'update': { lucide: 'lucide:arrow-up-circle', tabler: 'tabler:circle-arrow-up' },
  'down': { lucide: 'lucide:arrow-down-to-line', tabler: 'tabler:arrow-down' },
  'more': { lucide: 'lucide:more-horizontal', tabler: 'tabler:dots' },
  'document': { lucide: 'lucide:file-text', tabler: 'tabler:file-text' },
  'help': { lucide: 'lucide:help-circle', tabler: 'tabler:help-circle' },
  'path': { lucide: 'lucide:folder-open', tabler: 'tabler:folder-open' },
}

export const useIconStore = defineStore('icons', () => {
  // 当前图标库偏好
  const iconSet = ref(localStorage.getItem('iconSet') || 'lucide')
  
  // 获取当前图标集的组件名称
  const getIconComponent = (iconName) => {
    const mapping = iconMapping[iconName]
    if (!mapping) {
      // 如果没有映射，使用通用前缀
      return `Icon${iconSet.value.charAt(0).toUpperCase() + iconSet.value.slice(1)}${toPascalCase(iconName)}`
    }
    const iconId = mapping[iconSet.value]
    if (!iconId) return null
    
    // 解析图标ID (格式: "collection:icon-name")
    const [collection, name] = iconId.split(':')
    return `Icon${toPascalCase(collection)}${toPascalCase(name)}`
  }
  
  // 设置图标库
  const setIconSet = (set) => {
    if (set === 'lucide' || set === 'tabler') {
      iconSet.value = set
      localStorage.setItem('iconSet', set)
    }
  }
  
  // 切换图标库
  const toggleIconSet = () => {
    const newSet = iconSet.value === 'lucide' ? 'tabler' : 'lucide'
    setIconSet(newSet)
    return newSet
  }
  
  // 初始化
  const init = () => {
    const saved = localStorage.getItem('iconSet')
    if (saved && (saved === 'lucide' || saved === 'tabler')) {
      iconSet.value = saved
    }
  }
  
  return {
    iconSet: computed(() => iconSet.value),
    getIconComponent,
    setIconSet,
    toggleIconSet,
    init
  }
})

// 辅助函数：将 kebab-case 转换为 PascalCase
function toPascalCase(str) {
  return str
    .split('-')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join('')
}
