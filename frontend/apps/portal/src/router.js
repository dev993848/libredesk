export const routes = [
  {
    path: '/login',
    name: 'login',
    component: () => import('./views/LoginView.vue'),
  },
  {
    path: '/',
    component: () => import('./layouts/PortalLayout.vue'),
    meta: { auth: true },
    children: [
      {
        path: '',
        name: 'conversations',
        component: () => import('./views/ConversationsView.vue'),
      },
      {
        path: 'c/:uuid',
        name: 'conversation',
        component: () => import('./views/ConversationView.vue'),
      },
      {
        path: 'new',
        name: 'new-conversation',
        component: () => import('./views/NewConversationView.vue'),
      },
      {
        path: 'profile',
        name: 'profile',
        component: () => import('./views/ProfileView.vue'),
      },
    ],
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]
