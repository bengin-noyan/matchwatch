import { createApp } from 'vue'
import { createPinia } from 'pinia'
import '@fontsource-variable/atkinson-hyperlegible-next'
import '@fontsource/caveat/500.css'
import './style.css'
import App from './App.vue'
import { router } from './router'
import { setUnauthorizedHandler } from './api'
import { useSession } from './session'

const app = createApp(App).use(createPinia()).use(router)

// An expired session sends the user back to the login screen, keeping where they were.
setUnauthorizedHandler(() => {
  useSession().expired()
  const here = router.currentRoute.value
  if (here.name !== 'login') router.push({ name: 'login', query: { next: here.fullPath } })
})

app.mount('#app')
