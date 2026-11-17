import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './session'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/tanitim', name: 'landing', component: () => import('./views/LandingView.vue'), meta: { public: true } },
    { path: '/giris', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
    { path: '/kayit', name: 'signup', component: () => import('./views/SignupView.vue'), meta: { public: true } },
    // davet ve sıfırlama bağlantıları token'ı URL'nin # kısmında taşır; o kısım hiçbir sunucu loguna düşmez
    { path: '/davet', name: 'invite', component: () => import('./views/PasswordLinkView.vue'), meta: { public: true } },
    { path: '/sifremi-unuttum', name: 'forgot', component: () => import('./views/ForgotView.vue'), meta: { public: true } },
    { path: '/sifre', name: 'reset', component: () => import('./views/PasswordLinkView.vue'), meta: { public: true } },
    { path: '/', name: 'dashboard', component: () => import('./views/DashboardView.vue') },
    { path: '/faturalar', name: 'invoices', component: () => import('./views/InvoicesView.vue') },
    { path: '/bulgular', name: 'findings', component: () => import('./views/FindingsView.vue') },
    { path: '/faturalar/:id', name: 'invoice', component: () => import('./views/InvoiceView.vue'), props: true },
    { path: '/yukle', name: 'upload', component: () => import('./views/UploadView.vue') },
    { path: '/taslaklar', name: 'drafts', component: () => import('./views/DraftsView.vue') },
    { path: '/taslaklar/:id', name: 'draft', component: () => import('./views/DraftView.vue'), props: true },
    { path: '/eslesmeler', name: 'aliases', component: () => import('./views/AliasesView.vue') },
    { path: '/kullanicilar', name: 'users', component: () => import('./views/UsersView.vue') },
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
  // ön kapıya ilk gelen ürünü görür; doğrudan bir sayfaya gelen bağlantı ise giriş formuna gider
  if (!to.meta.public && !session.me) return to.fullPath === '/' ? { name: 'landing' } : { name: 'login', query: { next: to.fullPath } }
  if ((to.name === 'login' || to.name === 'signup') && session.me) return { name: 'dashboard' }
})
