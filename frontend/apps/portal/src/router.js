export const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('./views/LoginView.vue'),
  },
  {
    path: '/',
    name: 'conversations',
    component: () => import('./views/ConversationsView.vue'),
    meta: { auth: true },
  },
  {
    path: '/c/:uuid',
    name: 'conversation',
    component: () => import('./views/ConversationView.vue'),
    meta: { auth: true },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]
