import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './session'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/giris', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
    { path: '/', name: 'invoices', component: () => import('./views/InvoicesView.vue') },
    { path: '/faturalar/:id', name: 'invoice', component: () => import('./views/InvoiceView.vue'), props: true },
    { path: '/yukle', name: 'upload', component: () => import('./views/UploadView.vue') },
    { path: '/taslaklar', name: 'drafts', component: () => import('./views/DraftsView.vue') },
    { path: '/taslaklar/:id', name: 'draft', component: () => import('./views/DraftView.vue'), props: true },
    { path: '/ozet', name: 'summary', component: () => import('./views/SummaryView.vue') },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach(async (to) => {
  const session = useSession()
  if (!session.checked) await session.load()
  if (!to.meta.public && !session.me) return { name: 'login', query: to.fullPath === '/' ? {} : { next: to.fullPath } }
  if (to.name === 'login' && session.me) return { name: 'invoices' }
})
