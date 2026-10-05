import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from '@/router/index.js'
import App from '@/App.vue'
import '@/assets/css/tailwind.css'
import { vRipple } from '@/directives/ripple.js'

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.directive('ripple', vRipple)
app.mount('#app')
