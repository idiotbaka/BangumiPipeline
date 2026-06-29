import { createRouter, createWebHistory } from 'vue-router'
import { APIError, api } from './api'
import { session } from './session'
import SetupPage from './pages/SetupPage.vue'
import LoginPage from './pages/LoginPage.vue'
import DashboardPage from './pages/DashboardPage.vue'
import ScheduledTasksPage from './pages/ScheduledTasksPage.vue'
import SettingsPage from './pages/SettingsPage.vue'
import SystemLogsPage from './pages/SystemLogsPage.vue'
import AnimeListPage from './pages/AnimeListPage.vue'
import AnimeDetailPage from './pages/AnimeDetailPage.vue'
import SubscriptionMatchesPage from './pages/SubscriptionMatchesPage.vue'
import DownloadManagementPage from './pages/DownloadManagementPage.vue'
import TranscodeManagementPage from './pages/TranscodeManagementPage.vue'
import ViewerUserManagementPage from './pages/ViewerUserManagementPage.vue'
import ViewerSiteSettingsPage from './pages/ViewerSiteSettingsPage.vue'
import ViewerInviteManagementPage from './pages/ViewerInviteManagementPage.vue'
import ViewerCarouselManagementPage from './pages/ViewerCarouselManagementPage.vue'
import ViewerFilterManagementPage from './pages/ViewerFilterManagementPage.vue'
import AdminLayout from './components/AdminLayout.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: SetupPage },
    { path: '/login', name: 'login', component: LoginPage },
    {
      path: '/',
      component: AdminLayout,
      meta: { requiresAuth: true },
      children: [
        { path: '', redirect: '/dashboard' },
        { path: 'dashboard', name: 'dashboard', component: DashboardPage },
        { path: 'scheduled-tasks', name: 'scheduled-tasks', component: ScheduledTasksPage },
        { path: 'anime', name: 'anime', component: AnimeListPage },
        { path: 'anime/:bangumiId', name: 'anime-detail', component: AnimeDetailPage },
        { path: 'subscriptions', name: 'subscriptions', component: SubscriptionMatchesPage },
        { path: 'downloads', name: 'downloads', component: DownloadManagementPage },
        { path: 'transcodes', name: 'transcodes', component: TranscodeManagementPage },
        { path: 'system-logs', name: 'system-logs', component: SystemLogsPage },
        { path: 'settings', name: 'settings', component: SettingsPage },
        { path: 'viewer-users', name: 'viewer-users', component: ViewerUserManagementPage },
        { path: 'viewer-invites', name: 'viewer-invites', component: ViewerInviteManagementPage },
        { path: 'viewer-site-settings', name: 'viewer-site-settings', component: ViewerSiteSettingsPage },
        { path: 'viewer-carousels', name: 'viewer-carousels', component: ViewerCarouselManagementPage },
        { path: 'viewer-filters', name: 'viewer-filters', component: ViewerFilterManagementPage },
      ],
    },
    { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
  ],
})

router.beforeEach(async (to) => {
  try {
    const { initialized } = await api.setupStatus()
    if (!initialized) {
      return to.name === 'setup' ? true : { name: 'setup' }
    }
    if (to.name === 'setup') {
      return { name: 'login' }
    }

    if (to.meta.requiresAuth || to.name === 'login') {
      try {
        const { user } = await api.me()
        session.user = user
        if (to.name === 'login') {
          return { name: 'dashboard' }
        }
      } catch (error) {
        session.user = null
        if (error instanceof APIError && error.status === 401) {
          return to.meta.requiresAuth ? { name: 'login' } : true
        }
        throw error
      }
    }
    return true
  } catch {
    return true
  }
})

export default router
