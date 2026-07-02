import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import router from './router'
import { applyTheme } from './theme'

applyTheme('aria2mx', 'light')

// 创建Vue应用实例
const app = createApp(App)

// 使用路由
app.use(router)

// 挂载应用
app.mount('#app')
