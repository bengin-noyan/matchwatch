<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSession } from '../session'

const session = useSession()
const router = useRouter()
const route = useRoute()

// Telefonda menü bir düğmenin arkasında durur; bir sayfaya geçince kendiliğinden kapanır. Geniş ekranda hep açıktır.
const open = ref(false)
watch(() => route.fullPath, () => (open.value = false))

const nav = [
  { to: '/', label: 'Masa', exact: true },
  { to: '/faturalar', label: 'Faturalar' },
  { to: '/bulgular', label: 'Bulgular' },
  { to: '/tedarikciler', label: 'Tedarikçiler' },
  { to: '/eslesmeler', label: 'Kalem eşleşmeleri' },
  { to: '/yukle', label: 'Belge yükle' },
  { to: '/taslaklar', label: 'Sözleşme taslakları' },
  { to: '/kullanicilar', label: 'Kullanıcılar' },
]

async function logout() {
  await session.logout()
  router.push({ name: 'login' })
}
</script>

<template>
  <div class="min-h-dvh md:grid md:grid-cols-[15rem_1fr]">
    <aside
      class="border-rule bg-paper flex flex-col gap-6 border-b px-4 py-3 md:sticky md:top-0 md:h-dvh md:border-r md:border-b-0 md:px-5 md:py-7"
      @keydown.esc="open = false"
    >
      <div class="flex items-center justify-between gap-4">
        <RouterLink to="/" class="text-ink flex items-center gap-2 text-xl font-semibold tracking-tight">
          <svg viewBox="0 0 32 32" class="size-6" aria-hidden="true">
            <path d="M7 17c2-6 15-9 18-3 2 5-6 10-12 9-5-1-8-3-6-6" fill="none" stroke="var(--color-pencil)" stroke-width="2.6" stroke-linecap="round" />
          </svg>
          matchwatch
        </RouterLink>
        <button
          type="button"
          class="btn-secondary px-3 py-1.5 text-sm md:hidden"
          aria-controls="ana-menu"
          :aria-expanded="open"
          @click="open = !open"
        >
          <svg viewBox="0 0 20 20" class="size-4" aria-hidden="true">
            <path v-if="open" d="M5 5l10 10M15 5L5 15" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
            <path v-else d="M3 6h14M3 10h14M3 14h14" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
          </svg>
          Menü
        </button>
      </div>
      <nav id="ana-menu" aria-label="Ana menü" class="-mx-1 flex-col gap-1 md:flex" :class="open ? 'flex' : 'hidden'">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="nav-link rounded-field text-ink-soft hover:text-ink px-3 py-2 whitespace-nowrap"
          :exact-active-class="item.exact ? 'is-active' : ''"
          :active-class="item.exact ? '' : 'is-active'"
        >
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="text-ink-soft border-rule mt-auto border-t pt-4 text-sm md:block md:border-t-0 md:pt-0" :class="open ? 'block' : 'hidden'">
        <p class="text-ink font-semibold">{{ session.me?.tenant_name }}</p>
        <p class="truncate">{{ session.me?.email }}</p>
        <button type="button" class="text-ballpoint mt-3 underline-offset-4 hover:underline" @click="logout">Çıkış yap</button>
      </div>
    </aside>
    <main class="mx-auto w-full max-w-[84rem] px-4 py-6 md:px-10 md:py-9">
      <slot />
    </main>
  </div>
</template>

<style scoped>
.nav-link.is-active {
  color: var(--color-ink);
  background: var(--color-sheet);
  box-shadow: inset 3px 0 0 var(--color-ink);
  font-weight: 600;
}
</style>
