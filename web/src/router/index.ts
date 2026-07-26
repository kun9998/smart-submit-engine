import { createRouter, createWebHistory } from 'vue-router'
import { authApi, getSessionToken } from '@/api/auth'
import { adminNavByPath, adminProfileNav } from '@/lib/admin-nav'

function routeMeta(path: string) {
  const item = adminNavByPath.get(path)
  if (!item) return {}
  return { title: item.title, description: item.description }
}

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/install', name: 'install', component: () => import('@/views/InstallView.vue'), meta: { guest: true } },
    { path: '/login', name: 'login', component: () => import('@/views/LoginView.vue'), meta: { guest: true } },
    { path: '/forgot-password', name: 'forgot-password', component: () => import('@/views/ForgotPasswordView.vue'), meta: { guest: true } },
    {
      path: '/',
      component: () => import('@/layouts/AdminLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          name: 'overview',
          component: () => import('@/views/EngineOverviewView.vue'),
          meta: routeMeta('/'),
        },
        {
          path: 'platforms',
          name: 'dashboard',
          component: () => import('@/views/DashboardView.vue'),
          meta: routeMeta('/platforms'),
        },
        {
          path: 'huoyuan',
          name: 'huoyuan',
          component: () => import('@/views/HuoyuanConfigView.vue'),
          meta: routeMeta('/huoyuan'),
        },
        {
          path: 'logs',
          name: 'logs',
          component: () => import('@/views/LogsView.vue'),
          meta: routeMeta('/logs'),
        },
        {
          path: 'docs',
          name: 'docs',
          component: () => import('@/views/DevDocsView.vue'),
          meta: routeMeta('/docs'),
        },
        {
          path: 'profile',
          name: 'profile',
          component: () => import('@/views/ProfileView.vue'),
          meta: {
            title: adminProfileNav.title,
            description: adminProfileNav.description,
          },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('@/views/SystemSettingsView.vue'),
          meta: routeMeta('/settings'),
        },
        {
          path: 'upgrade',
          name: 'upgrade',
          component: () => import('@/views/SystemUpgradeView.vue'),
          meta: routeMeta('/upgrade'),
        },
        {
          path: 'monitor',
          name: 'monitor',
          component: () => import('@/views/SystemMonitorView.vue'),
          meta: routeMeta('/monitor'),
        },
        {
          path: 'ops',
          name: 'ops',
          component: () => import('@/views/AiOpsView.vue'),
          meta: routeMeta('/ops'),
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  let status: Awaited<ReturnType<typeof authApi.status>>['data']
  try {
    const res = await authApi.status()
    status = res.data
  } catch {
    if (to.name === 'install') return true
    return { name: 'install' }
  }

  if (!status?.installed) {
    if (to.name !== 'install') return { name: 'install' }
    return true
  }

  if (to.name === 'install') {
    return status.logged_in ? { name: 'overview' } : { name: 'login' }
  }

  const loggedIn = status.logged_in && !!getSessionToken()

  if (to.meta.requiresAuth && !loggedIn) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }

  if (to.meta.guest && to.name === 'login' && loggedIn) {
    return { name: 'overview' }
  }

  if (to.meta.guest && to.name === 'forgot-password' && loggedIn) {
    return { name: 'overview' }
  }

  return true
})

export default router
