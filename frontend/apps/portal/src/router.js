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
    path: '/new',
    name: 'new-conversation',
    component: () => import('./views/NewConversationView.vue'),
    meta: { auth: true },
  },
  {
    path: '/profile',
    name: 'profile',
    component: () => import('./views/ProfileView.vue'),
    meta: { auth: true },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]
