import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import { createI18n } from 'vue-i18n'
import App from './App.vue'
import { routes } from './router.js'
import { api, appConfig, getLanguage, getPublicConfig } from './api.js'
import '@shared-ui/assets/styles/main.scss'
import './theme.css'

async function bootstrap() {
  // Load public app config and language pack before mounting so the first
  // render is already localized.
  try {
    const config = await getPublicConfig()
    appConfig.lang = config['app.lang'] || 'en-US'
    appConfig.siteName = config['app.site_name'] || 'Libredesk'
    appConfig.logoUrl = config['app.logo_url'] || ''
    appConfig.faviconUrl = config['app.favicon_url'] || ''
    appConfig.portalEnabled = config['portal.enabled'] !== false
  } catch (err) {
    console.error('error loading portal config', err)
  }

  // Apply portal branding before the first render.
  document.documentElement.lang = appConfig.lang
  document.title = appConfig.siteName || 'Libredesk'
  if (appConfig.faviconUrl) {
    let icon = document.querySelector('link[rel="icon"]')
    if (!icon) {
      icon = document.createElement('link')
      icon.rel = 'icon'
      document.head.appendChild(icon)
    }
    icon.href = appConfig.faviconUrl
  }

  let messages = {}
  try {
    messages = await getLanguage(appConfig.lang)
  } catch (err) {
    console.error('error loading portal language', err)
  }

  const i18n = createI18n({
    legacy: false,
    locale: appConfig.lang,
    fallbackLocale: 'en-US',
    messages: { [appConfig.lang]: messages },
  })

  const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes,
  })

  // Redirect anonymous visitors to the login page.
  router.beforeEach(async (to) => {
    if (!to.meta.auth) return true
    try {
      await api.get('/api/v1/portal/me')
      return true
    } catch (err) {
      return { name: 'login', query: { next: to.fullPath } }
    }
  })

  const app = createApp(App)
  app.use(i18n)
  app.use(router)
  app.mount('#app')
}

bootstrap()
