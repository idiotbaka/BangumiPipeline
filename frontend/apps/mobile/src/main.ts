import { createApp } from 'vue'

import App from './App.vue'
import { isTVApp } from './platform'
import './style.css'
import { installTVFocusNavigation } from './tvFocus'

document.documentElement.classList.toggle('bp-tv-app', isTVApp)
document.body.classList.toggle('bp-tv-app', isTVApp)
createApp(App).mount('#app')
installTVFocusNavigation()
