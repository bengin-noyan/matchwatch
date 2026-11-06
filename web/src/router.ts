import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './session'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/tanitim', name: 'landing', component: () => import('./views/LandingView.vue'), meta: { public: true } },
    { path: '/giris', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
    { path: '/', name: 'dashboard', component: () => import('./views/DashboardView.vue') },
    { path: '/faturalar', name: 'invoices', component: () => import('./views/InvoicesView.vue') },
    { path: '/bulgular', name: 'findings', component: () => import('./views/FindingsView.vue') },
    { path: '/faturalar/:id', name: 'invoice', component: () => import('./views/InvoiceView.vue'), props: true },
    { path: '/yukle', name: 'upload', component: () => import('./views/UploadView.vue') },
    { path: '/taslaklar', name: 'drafts', component: () => import('./views/DraftsView.vue') },
    { path: '/taslaklar/:id', name: 'draft', component: () => import('./views/DraftView.vue'), props: true },
    { path: '/eslesmeler', name: 'aliases', component: () => import('./views/AliasesView.vue') },
    { path: '/tedarikciler', name: 'vendors', component: () => import('./views/VendorsView.vue') },
    { path: '/tedarikciler/:id', name: 'vendor', component: () => import('./views/VendorView.vue'), props: true },
    { path: '/ozet', redirect: '/' },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach(async (to) => {
  const session = useSession()
  if (!session.checked) await session.load()
  // a first visit to the front door shows the product; a deep link goes straight to the login form
  if (!to.meta.public && !session.me) return to.fullPath === '/' ? { name: 'landing' } : { name: 'login', query: { next: to.fullPath } }
  if (to.name === 'login' && session.me) return { name: 'dashboard' }
})
