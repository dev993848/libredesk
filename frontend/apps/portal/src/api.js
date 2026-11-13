import axios from 'axios'

// Public app config is loaded once at startup and shared with the views.
export const appConfig = {
  lang: 'en-US',
  siteName: 'Libredesk',
  logoUrl: '',
  faviconUrl: '',
  portalEnabled: true,
}

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
})

function cookie(name) {
  const match = document.cookie.match(new RegExp('(?:^|;\\s*)' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : ''
}

api.interceptors.request.use((config) => {
  const token = cookie('csrf_token')
  if (token) {
    config.headers = config.headers || {}
    config.headers['X-CSRFTOKEN'] = token
  }
  return config
})

export const getPublicConfig = async () => {
  const resp = await api.get('/api/v1/config')
  return resp.data.data || {}
}

export const getLanguage = async (lang) => {
  const resp = await api.get(`/api/v1/lang/${encodeURIComponent(lang)}`)
  return resp.data.data || {}
}

export const requestMagicLink = (email) => api.post('/api/v1/portal/auth/magic-link', { email })
export const logout = () => api.post('/api/v1/portal/auth/logout')
export const getMe = () => api.get('/api/v1/portal/me')
export const listConversations = (params) => api.get('/api/v1/portal/conversations', { params })
export const createConversation = (data) => api.post('/api/v1/portal/conversations', data)
export const getConversation = (uuid) => api.get(`/api/v1/portal/conversations/${uuid}`)
export const replyToConversation = (uuid, message) => api.post(`/api/v1/portal/conversations/${uuid}/messages`, { message })
export const uploadAttachment = (uuid, file) => {
  const form = new FormData()
  form.append('files', file)
  return api.post(`/api/v1/portal/conversations/${uuid}/attachments`, form)
}
export const markSeen = (uuid) => api.post(`/api/v1/portal/conversations/${uuid}/seen`)
