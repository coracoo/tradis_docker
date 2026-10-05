<template>
  <component 
    :is="iconComponent" 
    :class="iconClass"
    v-bind="$attrs"
  />
</template>

<script setup>
import { computed } from 'vue'
import { useIconStore } from '@/stores/icons.js'

// ===== Lucide 图标导入 =====
import IconLucideActivity from '~icons/lucide/activity'
import IconLucideArchive from '~icons/lucide/archive'
import IconLucideArchiveRestore from '~icons/lucide/archive-restore'
import IconLucideArrowDown from '~icons/lucide/arrow-down'
import IconLucideArrowLeft from '~icons/lucide/arrow-left'
import IconLucideArrowRight from '~icons/lucide/arrow-right'
import IconLucideArrowUp from '~icons/lucide/arrow-up'
import IconLucideArrowUpCircle from '~icons/lucide/circle-arrow-up'
import IconLucideArrowUpDown from '~icons/lucide/arrow-up-down'
import IconLucideAward from '~icons/lucide/award'
import IconLucideBarChart3 from '~icons/lucide/bar-chart-3'
import IconLucideBattery from '~icons/lucide/battery'
import IconLucideBell from '~icons/lucide/bell'
import IconLucideBluetooth from '~icons/lucide/bluetooth'
import IconLucideBook from '~icons/lucide/book'
import IconLucideBookOpen from '~icons/lucide/book-open'
import IconLucideBookmark from '~icons/lucide/bookmark'
import IconLucideBot from '~icons/lucide/bot'
import IconLucideBox from '~icons/lucide/box'
import IconLucideBoxes from '~icons/lucide/boxes'
import IconLucideBrush from '~icons/lucide/brush'
import IconLucidePackage from '~icons/lucide/package'
import IconLucideBug from '~icons/lucide/bug'
import IconLucideCalendar from '~icons/lucide/calendar'
import IconLucideCheck from '~icons/lucide/check'
import IconLucideChevronDown from '~icons/lucide/chevron-down'
import IconLucideChevronLeft from '~icons/lucide/chevron-left'
import IconLucideChevronRight from '~icons/lucide/chevron-right'
import IconLucideChevronUp from '~icons/lucide/chevron-up'
import IconLucideCircleAlert from '~icons/lucide/circle-alert'
import IconLucideCircleCheck from '~icons/lucide/circle-check'
import IconLucideCircleHelp from '~icons/lucide/circle-help'
import IconLucideClipboardPaste from '~icons/lucide/clipboard-paste'
import IconLucideClock from '~icons/lucide/clock'
import IconLucideCloud from '~icons/lucide/cloud'
import IconLucideCloudDownload from '~icons/lucide/cloud-download'
import IconLucideCloudOff from '~icons/lucide/cloud-off'
import IconLucideCloudUpload from '~icons/lucide/cloud-upload'
import IconLucideCode from '~icons/lucide/code'
import IconLucideCodeXml from '~icons/lucide/code-xml'
import IconLucideCheckCheck from '~icons/lucide/check-check'
import IconLucideCheckSquare from '~icons/lucide/check-square'
import IconLucideInbox from '~icons/lucide/inbox'
import IconLucideLoader2 from '~icons/lucide/loader-2'
import IconLucideCompass from '~icons/lucide/compass'
import IconLucideContainer from '~icons/lucide/container'
import IconLucideCopy from '~icons/lucide/copy'
import IconLucideCpu from '~icons/lucide/cpu'
import IconLucideCrosshair from '~icons/lucide/crosshair'
import IconLucideCrown from '~icons/lucide/crown'
import IconLucideDatabase from '~icons/lucide/database'
import IconLucideDiamond from '~icons/lucide/diamond'
import IconLucideDownload from '~icons/lucide/download'
import IconLucideEllipsisVertical from '~icons/lucide/ellipsis-vertical'
import IconLucideEraser from '~icons/lucide/eraser'
import IconLucideExternalLink from '~icons/lucide/external-link'
import IconLucideEye from '~icons/lucide/eye'
import IconLucideEyeOff from '~icons/lucide/eye-off'
import IconLucideFile from '~icons/lucide/file'
import IconLucideFileCode from '~icons/lucide/file-code'
import IconLucideFileStack from '~icons/lucide/file-stack'
import IconLucideFileText from '~icons/lucide/file-text'
import IconLucideFilter from '~icons/lucide/filter'
import IconLucideFlag from '~icons/lucide/flag'
import IconLucideFolder from '~icons/lucide/folder'
import IconLucideFolderMinus from '~icons/lucide/folder-minus'
import IconLucideFolderOpen from '~icons/lucide/folder-open'
import IconLucideFolderPlus from '~icons/lucide/folder-plus'
import IconLucideGem from '~icons/lucide/gem'
import IconLucideGift from '~icons/lucide/gift'
import IconLucideGitBranch from '~icons/lucide/git-branch'
import IconLucideGitCommitHorizontal from '~icons/lucide/git-commit-horizontal'
import IconLucideGitMerge from '~icons/lucide/git-merge'
import IconLucideGitPullRequest from '~icons/lucide/git-pull-request'
import IconLucideGithub from '~icons/lucide/github'
import IconLucideGitlab from '~icons/lucide/gitlab'
import IconLucideGlobe from '~icons/lucide/globe'
import IconLucideGraduationCap from '~icons/lucide/graduation-cap'
import IconLucideHardDrive from '~icons/lucide/hard-drive'
import IconLucideHeart from '~icons/lucide/heart'
import IconLucideHexagon from '~icons/lucide/hexagon'
import IconLucideHouse from '~icons/lucide/house'
import IconLucideImage from '~icons/lucide/image'
import IconLucideInfo from '~icons/lucide/info'
import IconLucideKey from '~icons/lucide/key'
import IconLucideLayers from '~icons/lucide/layers'
import IconLucideLayoutDashboard from '~icons/lucide/layout-dashboard'
import IconLucideLayoutGrid from '~icons/lucide/layout-grid'
import IconLucideLayoutTemplate from '~icons/lucide/layout-template'
import IconLucideLink from '~icons/lucide/link'
import IconLucideList from '~icons/lucide/list'
import IconLucideLocate from '~icons/lucide/locate'
import IconLucideLocateFixed from '~icons/lucide/locate-fixed'
import IconLucideLock from '~icons/lucide/lock'
import IconLucideLockOpen from '~icons/lucide/lock-open'
import IconLucideMail from '~icons/lucide/mail'
import IconLucideMap from '~icons/lucide/map'
import IconLucideMapPin from '~icons/lucide/map-pin'
import IconLucideMaximize from '~icons/lucide/maximize'
import IconLucideMaximize2 from '~icons/lucide/maximize-2'
import IconLucideMedal from '~icons/lucide/medal'
import IconLucideMenu from '~icons/lucide/menu'
import IconLucideMessageCircle from '~icons/lucide/message-circle'
import IconLucideMessageSquare from '~icons/lucide/message-square'
import IconLucideMinimize from '~icons/lucide/minimize'
import IconLucideMinimize2 from '~icons/lucide/minimize-2'
import IconLucideMinus from '~icons/lucide/minus'
import IconLucideMoon from '~icons/lucide/moon'
import IconLucideNavigation from '~icons/lucide/navigation'
import IconLucideNavigation2 from '~icons/lucide/navigation-2'
import IconLucideNetwork from '~icons/lucide/network'
import IconLucidePalette from '~icons/lucide/palette'
import IconLucidePanelLeft from '~icons/lucide/panel-left'
import IconLucidePause from '~icons/lucide/pause'
import IconLucidePenTool from '~icons/lucide/pen-tool'
import IconLucidePencil from '~icons/lucide/pencil'
import IconLucidePhone from '~icons/lucide/phone'
import IconLucidePin from '~icons/lucide/pin'
import IconLucidePlay from '~icons/lucide/play'
import IconLucidePlus from '~icons/lucide/plus'
import IconLucidePower from '~icons/lucide/power'
import IconLucideRefreshCw from '~icons/lucide/refresh-cw'
import IconLucideRocket from '~icons/lucide/rocket'
import IconLucideRotateCw from '~icons/lucide/rotate-cw'
import IconLucideRuler from '~icons/lucide/ruler'
import IconLucideScissors from '~icons/lucide/scissors'
import IconLucideScrollText from '~icons/lucide/scroll-text'
import IconLucideSearch from '~icons/lucide/search'
import IconLucideSend from '~icons/lucide/send'
import IconLucideServer from '~icons/lucide/server'
import IconLucideServerCog from '~icons/lucide/server-cog'
import IconLucideSettings from '~icons/lucide/settings'
import IconLucideShare from '~icons/lucide/share'
import IconLucideScale from '~icons/lucide/scale'
import IconLucideShare2 from '~icons/lucide/share-2'
import IconLucideShield from '~icons/lucide/shield'
import IconLucideSearchX from '~icons/lucide/search-x'
import IconLucideShoppingCart from '~icons/lucide/shopping-cart'
import IconLucideSparkles from '~icons/lucide/sparkles'
import IconLucideSquare from '~icons/lucide/square'
import IconLucideStar from '~icons/lucide/star'
import IconLucideStore from '~icons/lucide/store'
import IconLucideSun from '~icons/lucide/sun'
import IconLucideSunMoon from '~icons/lucide/sun-moon'
import IconLucideTable from '~icons/lucide/table'
import IconLucideTag from '~icons/lucide/tag'
import IconLucideTarget from '~icons/lucide/target'
import IconLucideTerminal from '~icons/lucide/terminal'
import IconLucideTrash2 from '~icons/lucide/trash-2'
import IconLucideTriangleAlert from '~icons/lucide/triangle-alert'
import IconLucideTrophy from '~icons/lucide/trophy'
import IconLucideUnlink from '~icons/lucide/unlink'
import IconLucideUpload from '~icons/lucide/upload'
import IconLucideUser from '~icons/lucide/user'
import IconLucideVolume2 from '~icons/lucide/volume-2'
import IconLucideVolumeX from '~icons/lucide/volume-x'
import IconLucideWifi from '~icons/lucide/wifi'
import IconLucideX from '~icons/lucide/x'
import IconLucideZap from '~icons/lucide/zap'

// ===== Simple Icons 导入 =====
import IconSimpleDocker from '~icons/simple-icons/docker'

// ===== Tabler 图标导入 =====
import IconTablerActivity from '~icons/tabler/activity'
import IconTablerAlertCircle from '~icons/tabler/alert-circle'
import IconTablerAlertTriangle from '~icons/tabler/alert-triangle'
import IconTablerApps from '~icons/tabler/apps'
import IconTablerArchive from '~icons/tabler/archive'
import IconTablerArrowDown from '~icons/tabler/arrow-down'
import IconTablerArrowLeft from '~icons/tabler/arrow-left'
import IconTablerArrowRight from '~icons/tabler/arrow-right'
import IconTablerArrowUp from '~icons/tabler/arrow-up'
import IconTablerArrowsSort from '~icons/tabler/arrows-sort'
import IconTablerAward from '~icons/tabler/award'
import IconTablerBattery from '~icons/tabler/battery'
import IconTablerBell from '~icons/tabler/bell'
import IconTablerBluetooth from '~icons/tabler/bluetooth'
import IconTablerBolt from '~icons/tabler/bolt'
import IconTablerBook from '~icons/tabler/book'
import IconTablerBookmark from '~icons/tabler/bookmark'
import IconTablerBox from '~icons/tabler/box'
import IconTablerBoxes from '~icons/tabler/box-multiple'
import IconTablerBrandDocker from '~icons/tabler/brand-docker'
import IconTablerBrandGithub from '~icons/tabler/brand-github'
import IconTablerBrandGitlab from '~icons/tabler/brand-gitlab'
import IconTablerBrush from '~icons/tabler/brush'
import IconTablerBug from '~icons/tabler/bug'
import IconTablerBuildingStore from '~icons/tabler/building-store'
import IconTablerCalendar from '~icons/tabler/calendar'
import IconTablerChartBar from '~icons/tabler/chart-bar'
import IconTablerCheck from '~icons/tabler/check'
import IconTablerChevronDown from '~icons/tabler/chevron-down'
import IconTablerChevronLeft from '~icons/tabler/chevron-left'
import IconTablerChevronRight from '~icons/tabler/chevron-right'
import IconTablerChevronUp from '~icons/tabler/chevron-up'
import IconTablerCircleCheck from '~icons/tabler/circle-check'
import IconTablerClipboardText from '~icons/tabler/clipboard-text'
import IconTablerClock from '~icons/tabler/clock'
import IconTablerCloud from '~icons/tabler/cloud'
import IconTablerCloudDownload from '~icons/tabler/cloud-download'
import IconTablerCloudOff from '~icons/tabler/cloud-off'
import IconTablerCloudUpload from '~icons/tabler/cloud-upload'
import IconTablerCode from '~icons/tabler/code'
import IconTablerChecks from '~icons/tabler/checks'
import IconTablerCheckbox from '~icons/tabler/checkbox'

import IconTablerInbox from '~icons/tabler/inbox'
import IconTablerLoader2 from '~icons/tabler/loader-2'
import IconTablerCompass from '~icons/tabler/compass'
import IconTablerContainer from '~icons/tabler/container'
import IconTablerCopy from '~icons/tabler/copy'
import IconTablerCpu from '~icons/tabler/cpu'
import IconTablerCrosshair from '~icons/tabler/crosshair'
import IconTablerCrown from '~icons/tabler/crown'
import IconTablerCube from '~icons/tabler/cube'
import IconTablerCubes from '~icons/tabler/box-multiple'
import IconTablerDatabase from '~icons/tabler/database'
import IconTablerLayers from '~icons/tabler/stack'  // tabler uses 'stack' for layers
import IconTablerPackage from '~icons/tabler/package'
import IconTablerDeviceAnalytics from '~icons/tabler/device-analytics'
import IconTablerDiamond from '~icons/tabler/diamond'
import IconTablerDotsVertical from '~icons/tabler/dots-vertical'
import IconTablerDownload from '~icons/tabler/download'
import IconTablerEdit from '~icons/tabler/edit'
import IconTablerEraser from '~icons/tabler/eraser'
import IconTablerExternalLink from '~icons/tabler/external-link'
import IconTablerEye from '~icons/tabler/eye'
import IconTablerEyeOff from '~icons/tabler/eye-off'
import IconTablerFile from '~icons/tabler/file'
import IconTablerFileCode from '~icons/tabler/file-code'
import IconTablerFileText from '~icons/tabler/file-text'
import IconTablerFilter from '~icons/tabler/filter'
import IconTablerFlag from '~icons/tabler/flag'
import IconTablerFolder from '~icons/tabler/folder'
import IconTablerFolderMinus from '~icons/tabler/folder-minus'
import IconTablerFolderOpen from '~icons/tabler/folder-open'
import IconTablerFolderPlus from '~icons/tabler/folder-plus'
import IconTablerGift from '~icons/tabler/gift'
import IconTablerGitBranch from '~icons/tabler/git-branch'
import IconTablerGitCommit from '~icons/tabler/git-commit'
import IconTablerGitMerge from '~icons/tabler/git-merge'
import IconTablerGitPullRequest from '~icons/tabler/git-pull-request'
import IconTablerGlobe from '~icons/tabler/globe'
import IconTablerHeart from '~icons/tabler/heart'
import IconTablerHexagon from '~icons/tabler/hexagon'
import IconTablerHome from '~icons/tabler/home'
import IconTablerInfoCircle from '~icons/tabler/info-circle'
import IconTablerKey from '~icons/tabler/key'
import IconTablerLayoutDashboard from '~icons/tabler/layout-dashboard'
import IconTablerLayoutGrid from '~icons/tabler/layout-grid'
import IconTablerLayoutSidebar from '~icons/tabler/layout-sidebar'
import IconTablerLink from '~icons/tabler/link'
import IconTablerLinkOff from '~icons/tabler/link-off'
import IconTablerList from '~icons/tabler/list'
import IconTablerLocate from '~icons/tabler/map-pin'
import IconTablerLock from '~icons/tabler/lock'
import IconTablerLockOpen from '~icons/tabler/lock-open'
import IconTablerMail from '~icons/tabler/mail'
import IconTablerMap from '~icons/tabler/map'
import IconTablerMapPin from '~icons/tabler/map-pin'
import IconTablerMaximize from '~icons/tabler/maximize'
import IconTablerMedal from '~icons/tabler/medal'
import IconTablerMenu from '~icons/tabler/menu'
import IconTablerMessage from '~icons/tabler/message'
import IconTablerMessageCircle from '~icons/tabler/message-circle'
import IconTablerMessages from '~icons/tabler/messages'
import IconTablerMinimize from '~icons/tabler/minimize'
import IconTablerMinus from '~icons/tabler/minus'
import IconTablerMoon from '~icons/tabler/moon'
import IconTablerNavigation from '~icons/tabler/navigation'
import IconTablerNetwork from '~icons/tabler/network'
import IconTablerPalette from '~icons/tabler/palette'
import IconTablerPencil from '~icons/tabler/pencil'
import IconTablerPhone from '~icons/tabler/phone'
import IconTablerPhoto from '~icons/tabler/photo'
import IconTablerPin from '~icons/tabler/pin'
import IconTablerPlayerPlay from '~icons/tabler/player-play'
import IconTablerPlayerPause from '~icons/tabler/player-pause'
import IconTablerPlus from '~icons/tabler/plus'
import IconTablerPower from '~icons/tabler/power'
import IconTablerRefresh from '~icons/tabler/refresh'
import IconTablerRobot from '~icons/tabler/robot'
import IconTablerRocket from '~icons/tabler/rocket'
import IconTablerRuler from '~icons/tabler/ruler'
import IconTablerScale from '~icons/tabler/scale'
import IconTablerSchool from '~icons/tabler/school'
import IconTablerScissors from '~icons/tabler/scissors'
import IconTablerSearch from '~icons/tabler/search'
import IconTablerSearchOff from '~icons/tabler/search-off'
import IconTablerShoppingCart from '~icons/tabler/shopping-cart'
import IconTablerSend from '~icons/tabler/send'
import IconTablerServer from '~icons/tabler/server'
import IconTablerSettings from '~icons/tabler/settings'
import IconTablerShare from '~icons/tabler/share'
import IconTablerShare2 from '~icons/tabler/share-2'
import IconTablerShield from '~icons/tabler/shield'
import IconTablerSparkles from '~icons/tabler/sparkles'
import IconTablerSquare from '~icons/tabler/square'
import IconTablerStack from '~icons/tabler/stack'
import IconTablerStar from '~icons/tabler/star'
import IconTablerSun from '~icons/tabler/sun'
import IconTablerTable from '~icons/tabler/table'
import IconTablerTag from '~icons/tabler/tag'
import IconTablerTarget from '~icons/tabler/target'
import IconTablerTerminal from '~icons/tabler/terminal'
import IconTablerTrash from '~icons/tabler/trash'
import IconTablerTrophy from '~icons/tabler/trophy'
import IconTablerUpload from '~icons/tabler/upload'
import IconTablerUser from '~icons/tabler/user'
import IconTablerVolume from '~icons/tabler/volume'
import IconTablerVolumeOff from '~icons/tabler/volume-off'
import IconTablerWifi from '~icons/tabler/wifi'
import IconTablerX from '~icons/tabler/x'

const props = defineProps({
  name: {
    type: String,
    required: true
  },
  size: {
    type: [String, Number],
    default: '20'
  },
  class: {
    type: String,
    default: ''
  }
})

const iconStore = useIconStore()

// Lucide 图标映射表
const lucideIconMap = {
  'box': IconLucideBox,
  'container': IconLucideContainer,
  'mdi-docker': IconLucideContainer,
  'mdi:docker': IconLucideContainer,
  'folder': IconLucideFolder,
  'image': IconLucideImage,
  'network': IconLucideNetwork,
  'database': IconLucideDatabase,
  'settings': IconLucideSettings,
  'dashboard': IconLucideLayoutDashboard,
  'containers': IconLucideContainer,
  'compose': IconLucideFileStack,
  'terminal': IconLucideTerminal,
  'logs': IconLucideScrollText,
  'start': IconLucidePlay,
  'play': IconLucidePlay,
  'pause': IconLucidePause,
  'stop': IconLucideSquare,
  'restart': IconLucideRotateCw,
  'refresh': IconLucideRefreshCw,
  'delete': IconLucideTrash2,
  'trash': IconLucideTrash2,
  'trash-2': IconLucideTrash2,
  'edit': IconLucidePencil,
  'pencil': IconLucidePencil,
  'search': IconLucideSearch,
  'search-x': IconLucideSearchX,
  'plus': IconLucidePlus,
  'add': IconLucidePlus,
  'minus': IconLucideMinus,
  'remove': IconLucideMinus,
  'close': IconLucideX,
  'x': IconLucideX,
  'check': IconLucideCheck,
  'check-check': IconLucideCheckCheck,
  'check-square': IconLucideCheckSquare,
  'inbox': IconLucideInbox,
  'loader-2': IconLucideLoader2,
  'chevron-down': IconLucideChevronDown,
  'chevron-up': IconLucideChevronUp,
  'chevron-left': IconLucideChevronLeft,
  'chevron-right': IconLucideChevronRight,
  'arrow-left': IconLucideArrowLeft,
  'arrow-right': IconLucideArrowRight,
  'arrow-up': IconLucideArrowUp,
  'arrow-down': IconLucideArrowDown,
  'arrow-up-down': IconLucideArrowUpDown,
  'more': IconLucideEllipsisVertical,
  'more-vertical': IconLucideEllipsisVertical,
  'download': IconLucideDownload,
  'upload': IconLucideUpload,
  'copy': IconLucideCopy,
  'paste': IconLucideClipboardPaste,
  'eye': IconLucideEye,
  'eye-off': IconLucideEyeOff,
  'sun': IconLucideSun,
  'moon': IconLucideMoon,
  'bell': IconLucideBell,
  'user': IconLucideUser,
  'lock': IconLucideLock,
  'unlock': IconLucideLockOpen,
  'lock-open': IconLucideLockOpen,
  'cpu': IconLucideCpu,
  'memory': IconLucideHardDrive,
  'disk': IconLucideDatabase,
  'grid': IconLucideLayoutGrid,
  'table': IconLucideTable,
  'list': IconLucideList,
  'filter': IconLucideFilter,
  'sort': IconLucideArrowUpDown,
  'tag': IconLucideTag,
  'info': IconLucideInfo,
  'warning': IconLucideTriangleAlert,
  'alert-triangle': IconLucideTriangleAlert,
  'triangle-alert': IconLucideTriangleAlert,
  'error': IconLucideCircleAlert,
  'alert-circle': IconLucideCircleAlert,
  'circle-alert': IconLucideCircleAlert,
  'success': IconLucideCircleCheck,
  'check-circle': IconLucideCircleCheck,
  'circle-check': IconLucideCircleCheck,
  'clock': IconLucideClock,
  'calendar': IconLucideCalendar,
  'home': IconLucideHouse,
  'house': IconLucideHouse,
  'external-link': IconLucideExternalLink,
  'link': IconLucideLink,
  'unlink': IconLucideUnlink,
  'menu': IconLucideMenu,
  'apps': IconLucideLayoutGrid,
  'store': IconLucideStore,
  'chart': IconLucideBarChart3,
  'activity': IconLucideActivity,
  'zap': IconLucideZap,
  'shield': IconLucideShield,
  'key': IconLucideKey,
  'mail': IconLucideMail,
  'phone': IconLucidePhone,
  'map': IconLucideMap,
  'globe': IconLucideGlobe,
  'wifi': IconLucideWifi,
  'bluetooth': IconLucideBluetooth,
  'battery': IconLucideBattery,
  'power': IconLucidePower,
  'volume': IconLucideVolume2,
  'volume-off': IconLucideVolumeX,
  'maximize': IconLucideMaximize,
  'minimize': IconLucideMinimize,
  'fullscreen': IconLucideMaximize2,
  'exit-fullscreen': IconLucideMinimize2,
  'sidebar': IconLucidePanelLeft,
  'layers': IconLucideLayers,
  'layer': IconLucideLayers,
  'stack': IconLucideLayers,
  'cube': IconLucideBox,
  'cubes': IconLucideBoxes,
  'package': IconLucidePackage,
  'server': IconLucideServer,
  'hard-drive': IconLucideHardDrive,
  'scale': IconLucideScale,
  'shopping-cart': IconLucideShoppingCart,
  'cloud': IconLucideCloud,
  'cloud-upload': IconLucideCloudUpload,
  'cloud-download': IconLucideCloudDownload,
  'cloud-off': IconLucideCloudOff,
  'backup': IconLucideArchiveRestore,
  'archive': IconLucideArchive,
  'file': IconLucideFile,
  'file-text': IconLucideFileText,
  'file-code': IconLucideFileCode,
  'folder-open': IconLucideFolderOpen,
  'folder-plus': IconLucideFolderPlus,
  'folder-minus': IconLucideFolderMinus,
  'git-branch': IconLucideGitBranch,
  'git-commit': IconLucideGitCommitHorizontal,
  'git-commit-horizontal': IconLucideGitCommitHorizontal,
  'git-merge': IconLucideGitMerge,
  'git-pull-request': IconLucideGitPullRequest,
  'github': IconLucideGithub,
  'gitlab': IconLucideGitlab,
  'docker': IconSimpleDocker,
  'kubernetes': IconLucideHexagon,
  'code': IconLucideCode,
  'code-2': IconLucideCodeXml,
  'code-xml': IconLucideCodeXml,
  'bug': IconLucideBug,
  'brush': IconLucideBrush,
  'palette': IconLucidePalette,
  'pen': IconLucidePenTool,
  'eraser': IconLucideEraser,
  'scissors': IconLucideScissors,
  'ruler': IconLucideRuler,
  'compass': IconLucideCompass,
  'map-pin': IconLucideMapPin,
  'navigation': IconLucideNavigation,
  'navigation-2': IconLucideNavigation2,
  'locate': IconLucideLocate,
  'locate-fixed': IconLucideLocateFixed,
  'aim': IconLucideTarget,
  'target': IconLucideTarget,
  'crosshair': IconLucideCrosshair,
  'send': IconLucideSend,
  'share': IconLucideShare,
  'share-2': IconLucideShare2,
  'message': IconLucideMessageSquare,
  'message-circle': IconLucideMessageCircle,
  'chat': IconLucideMessageSquare,
  'comment': IconLucideMessageCircle,
  'heart': IconLucideHeart,
  'star': IconLucideStar,
  'bookmark': IconLucideBookmark,
  'flag': IconLucideFlag,
  'pin': IconLucidePin,
  'book': IconLucideBook,
  'book-open': IconLucideBookOpen,
  'graduation-cap': IconLucideGraduationCap,
  'award': IconLucideAward,
  'medal': IconLucideMedal,
  'trophy': IconLucideTrophy,
  'gift': IconLucideGift,
  'crown': IconLucideCrown,
  'gem': IconLucideGem,
  'diamond': IconLucideDiamond,
  'rocket': IconLucideRocket,
  'sparkles': IconLucideSparkles,
  'bot': IconLucideBot,
  'robot': IconLucideBot,
  'ai': IconLucideBot,
}

// Tabler 图标映射表
const tablerIconMap = {
  'box': IconTablerBox,
  'container': IconTablerContainer,
  'mdi-docker': IconTablerBrandDocker,
  'mdi:docker': IconTablerBrandDocker,
  'folder': IconTablerFolder,
  'image': IconTablerPhoto,
  'network': IconTablerNetwork,
  'database': IconTablerDatabase,
  'settings': IconTablerSettings,
  'dashboard': IconTablerLayoutDashboard,
  'containers': IconTablerContainer,
  'compose': IconTablerStack,
  'terminal': IconTablerTerminal,
  'logs': IconTablerFileText,
  'start': IconTablerPlayerPlay,
  'play': IconTablerPlayerPlay,
  'pause': IconTablerPlayerPause,
  'stop': IconTablerSquare,
  'restart': IconTablerRefresh,
  'refresh': IconTablerRefresh,
  'delete': IconTablerTrash,
  'trash': IconTablerTrash,
  'trash-2': IconTablerTrash,
  'edit': IconTablerEdit,
  'pencil': IconTablerPencil,
  'search': IconTablerSearch,
  'search-x': IconTablerSearchOff,
  'plus': IconTablerPlus,
  'add': IconTablerPlus,
  'minus': IconTablerMinus,
  'remove': IconTablerMinus,
  'close': IconTablerX,
  'x': IconTablerX,
  'check': IconTablerCheck,
  'check-check': IconTablerChecks,
  'check-square': IconTablerCheckbox,
  'inbox': IconTablerInbox,
  'loader-2': IconTablerLoader2,
  'chevron-down': IconTablerChevronDown,
  'chevron-up': IconTablerChevronUp,
  'chevron-left': IconTablerChevronLeft,
  'chevron-right': IconTablerChevronRight,
  'arrow-left': IconTablerArrowLeft,
  'arrow-right': IconTablerArrowRight,
  'arrow-up': IconTablerArrowUp,
  'arrow-down': IconTablerArrowDown,
  'arrow-up-down': IconTablerArrowsSort,
  'more': IconTablerDotsVertical,
  'more-vertical': IconTablerDotsVertical,
  'download': IconTablerDownload,
  'upload': IconTablerUpload,
  'copy': IconTablerCopy,
  'paste': IconTablerClipboardText,
  'eye': IconTablerEye,
  'eye-off': IconTablerEyeOff,
  'sun': IconTablerSun,
  'moon': IconTablerMoon,
  'bell': IconTablerBell,
  'user': IconTablerUser,
  'lock': IconTablerLock,
  'unlock': IconTablerLockOpen,
  'lock-open': IconTablerLockOpen,
  'cpu': IconTablerCpu,
  'memory': IconTablerDeviceAnalytics,
  'disk': IconTablerDatabase,
  'grid': IconTablerLayoutGrid,
  'table': IconTablerTable,
  'list': IconTablerList,
  'filter': IconTablerFilter,
  'sort': IconTablerArrowsSort,
  'tag': IconTablerTag,
  'info': IconTablerInfoCircle,
  'warning': IconTablerAlertTriangle,
  'alert-triangle': IconTablerAlertTriangle,
  'triangle-alert': IconTablerAlertTriangle,
  'error': IconTablerAlertCircle,
  'alert-circle': IconTablerAlertCircle,
  'circle-alert': IconTablerAlertCircle,
  'success': IconTablerCircleCheck,
  'check-circle': IconTablerCircleCheck,
  'circle-check': IconTablerCircleCheck,
  'clock': IconTablerClock,
  'calendar': IconTablerCalendar,
  'home': IconTablerHome,
  'house': IconTablerHome,
  'external-link': IconTablerExternalLink,
  'link': IconTablerLink,
  'unlink': IconTablerLinkOff,
  'menu': IconTablerMenu,
  'apps': IconTablerApps,
  'store': IconTablerBuildingStore,
  'chart': IconTablerChartBar,
  'activity': IconTablerActivity,
  'zap': IconTablerBolt,
  'shield': IconTablerShield,
  'key': IconTablerKey,
  'mail': IconTablerMail,
  'phone': IconTablerPhone,
  'map': IconTablerMap,
  'globe': IconTablerGlobe,
  'wifi': IconTablerWifi,
  'bluetooth': IconTablerBluetooth,
  'battery': IconTablerBattery,
  'power': IconTablerPower,
  'volume': IconTablerVolume,
  'volume-off': IconTablerVolumeOff,
  'maximize': IconTablerMaximize,
  'minimize': IconTablerMinimize,
  'fullscreen': IconTablerMaximize,
  'exit-fullscreen': IconTablerMinimize,
  'sidebar': IconTablerLayoutSidebar,
  'layers': IconTablerLayers,
  'layer': IconTablerLayers,
  'stack': IconTablerStack,
  'cube': IconTablerCube,
  'cubes': IconTablerCubes,
  'package': IconTablerPackage,
  'server': IconTablerServer,
  'hard-drive': IconTablerDatabase,
  'scale': IconTablerScale,
  'shopping-cart': IconTablerShoppingCart,
  'cloud': IconTablerCloud,
  'cloud-upload': IconTablerCloudUpload,
  'cloud-download': IconTablerCloudDownload,
  'cloud-off': IconTablerCloudOff,
  'backup': IconTablerArchive,
  'archive': IconTablerArchive,
  'file': IconTablerFile,
  'file-text': IconTablerFileText,
  'file-code': IconTablerFileCode,
  'folder-open': IconTablerFolderOpen,
  'folder-plus': IconTablerFolderPlus,
  'folder-minus': IconTablerFolderMinus,
  'git-branch': IconTablerGitBranch,
  'git-commit': IconTablerGitCommit,
  'git-commit-horizontal': IconTablerGitCommit,
  'git-merge': IconTablerGitMerge,
  'git-pull-request': IconTablerGitPullRequest,
  'github': IconTablerBrandGithub,
  'gitlab': IconTablerBrandGitlab,
  'docker': IconSimpleDocker,
  'kubernetes': IconTablerHexagon,
  'code': IconTablerCode,
  'code-2': IconTablerCode,
  'code-xml': IconTablerCode,
  'bug': IconTablerBug,
  'brush': IconTablerBrush,
  'palette': IconTablerPalette,
  'pen': IconTablerPencil,
  'eraser': IconTablerEraser,
  'scissors': IconTablerScissors,
  'ruler': IconTablerRuler,
  'compass': IconTablerCompass,
  'map-pin': IconTablerMapPin,
  'navigation': IconTablerNavigation,
  'navigation-2': IconTablerNavigation,
  'locate': IconTablerLocate,
  'locate-fixed': IconTablerLocate,
  'aim': IconTablerTarget,
  'target': IconTablerTarget,
  'crosshair': IconTablerCrosshair,
  'send': IconTablerSend,
  'share': IconTablerShare,
  'share-2': IconTablerShare2,
  'message': IconTablerMessage,
  'message-circle': IconTablerMessageCircle,
  'chat': IconTablerMessages,
  'comment': IconTablerMessage,
  'heart': IconTablerHeart,
  'star': IconTablerStar,
  'bookmark': IconTablerBookmark,
  'flag': IconTablerFlag,
  'pin': IconTablerPin,
  'book': IconTablerBook,
  'book-open': IconTablerBook,
  'graduation-cap': IconTablerSchool,
  'award': IconTablerAward,
  'medal': IconTablerMedal,
  'trophy': IconTablerTrophy,
  'gift': IconTablerGift,
  'crown': IconTablerCrown,
  'gem': IconTablerDiamond,
  'diamond': IconTablerDiamond,
  'rocket': IconTablerRocket,
  'sparkles': IconTablerSparkles,
  'bot': IconTablerRobot,
  'robot': IconTablerRobot,
  'ai': IconTablerRobot,
}

const iconComponent = computed(() => {
  const name = props.name
  const iconSet = iconStore.iconSet
  
  // 处理 lucide:xxx 或 tabler:xxx 格式
  if (name && name.includes(':')) {
    const [collection, iconName] = name.split(':')
    if (collection === 'lucide') {
      return lucideIconMap[iconName] || IconLucideBox
    } else if (collection === 'tabler') {
      return tablerIconMap[iconName] || IconTablerBox
    }
  }
  
  // 根据当前图标集返回对应图标
  if (iconSet === 'tabler') {
    return tablerIconMap[name] || IconTablerBox
  }
  
  // 默认使用 Lucide
  return lucideIconMap[name] || IconLucideBox
})

const iconClass = computed(() => {
  return props.class || `icon-size-${props.size}`
})
</script>

<style scoped>
[class*="icon-size-"] {
  width: 1em;
  height: 1em;
  font-size: v-bind('size + "px"');
}
</style>
