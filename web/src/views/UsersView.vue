<script setup lang="ts">
// The company's logins and open invites. There is no mail delivery yet: an invite or reset makes a one-time link
// that the person here copies and hands over themselves.
import { onMounted, ref } from 'vue'
import { api, ApiError, type InviteRow, type UserRow } from '../api'
import { formatDateTime } from '../money'
import { useSession } from '../session'

const session = useSession()
const users = ref<UserRow[] | null>(null)
const invites = ref<InviteRow[]>([])
const email = ref('')
const error = ref('')
const busy = ref(false)
// the last link made: shown once, the server keeps only its hash
const link = ref<{ url: string; text: string } | null>(null)
const copied = ref(false)

const messages: Record<string, string> = {
  INVALID_EMAIL: 'E-posta adresi geçerli değil.',
  EMAIL_TAKEN: 'Bu e-postayla zaten bir hesap var.',
}
const fail = (e: unknown, fallback: string) => {
  error.value = e instanceof ApiError ? (messages[e.code] ?? e.message) : fallback
}

async function load() {
  try {
    const r = await api.users()
    users.value = r.users
    invites.value = r.invites
  } catch (e) {
    fail(e, 'Kullanıcılar yüklenemedi.')
  }
}
onMounted(load)

function show(path: string, token: string, text: string) {
  link.value = { url: `${location.origin}${path}#${token}`, text }
  copied.value = false
}

async function invite() {
  busy.value = true
  error.value = ''
  try {
    const r = await api.invite(email.value)
    show('/davet', r.token, `${r.email} için davet bağlantısı, ${formatDateTime(r.expires_at)} tarihine kadar bir kez kullanılabilir.`)
    email.value = ''
    await load()
  } catch (e) {
    fail(e, 'Davet oluşturulamadı.')
  } finally {
    busy.value = false
  }
}

async function reset(u: UserRow) {
  busy.value = true
  error.value = ''
  try {
    const r = await api.resetLink(u.id)
    show('/sifre', r.token, `${r.email} için şifre sıfırlama bağlantısı, ${formatDateTime(r.expires_at)} tarihine kadar bir kez kullanılabilir.`)
  } catch (e) {
    fail(e, 'Sıfırlama bağlantısı oluşturulamadı.')
  } finally {
    busy.value = false
  }
}

async function revoke(i: InviteRow) {
  busy.value = true
  error.value = ''
  try {
    await api.revokeInvite(i.id)
    link.value = null
    await load()
  } catch (e) {
    fail(e, 'Davet iptal edilemedi.')
  } finally {
    busy.value = false
  }
}

async function copy() {
  try {
    await navigator.clipboard.writeText(link.value!.url)
    copied.value = true
  } catch {
    /* no clipboard access (http, old browser): the field is selectable */
  }
}
</script>

<template>
  <header class="mb-6">
    <h1 class="text-3xl font-semibold tracking-tight">Kullanıcılar</h1>
    <p class="text-ink-soft mt-1 max-w-prose">
      Firmanın hesabına giriş yapabilen kişiler. Yeni birini davet ettiğinde ya da birinin şifresini sıfırladığında
      tek kullanımlık bir bağlantı oluşur; bağlantıyı kopyalayıp kişiye sen iletirsin.
    </p>
  </header>

  <p v-if="error" role="alert" class="text-pencil mb-4">{{ error }}</p>

  <section v-if="link" class="bg-sheet border-ballpoint rounded-sheet mb-6 border-2 p-5" aria-labelledby="link-title">
    <h2 id="link-title" class="text-ballpoint font-semibold">Bağlantı hazır</h2>
    <p class="text-ink-soft mt-1 text-sm">{{ link.text }} Bu bağlantı yalnızca şimdi gösterilir.</p>
    <div class="mt-3 flex flex-wrap gap-2">
      <input :value="link.url" readonly aria-label="Bağlantı" class="field num min-w-0 flex-1 text-sm" @focus="($event.target as HTMLInputElement).select()" />
      <button type="button" class="btn-primary" @click="copy">{{ copied ? 'Kopyalandı' : 'Kopyala' }}</button>
    </div>
  </section>

  <form class="bg-sheet border-rule rounded-sheet mb-6 flex flex-wrap items-end gap-3 border p-5" @submit.prevent="invite">
    <label class="min-w-0 flex-1">
      <span class="mb-1 block text-sm font-semibold">Davet edilecek kişinin e-postası</span>
      <input v-model="email" type="email" required maxlength="254" autocomplete="off" class="field" />
    </label>
    <button type="submit" class="btn-primary" :disabled="busy">Davet bağlantısı oluştur</button>
  </form>

  <template v-if="users">
    <div class="bg-sheet border-rule rounded-sheet overflow-x-auto border">
      <table class="w-full min-w-[36rem] border-collapse text-sm">
        <thead>
          <tr class="border-rule text-ink-soft border-b text-left">
            <th scope="col" class="px-4 py-3 font-semibold">E-posta</th>
            <th scope="col" class="px-4 py-3 font-semibold">Durum</th>
            <th scope="col" class="px-4 py-3"><span class="sr-only">İşlem</span></th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="u in users" :key="u.id" class="border-rule border-b">
            <td class="px-4 py-3 font-semibold">
              {{ u.email }} <span v-if="u.email === session.me?.email" class="text-ink-soft font-normal">(sen)</span>
            </td>
            <td class="text-ink-soft num px-4 py-3">{{ formatDateTime(u.created_at) }} tarihinden beri</td>
            <td class="px-4 py-3 text-right">
              <button type="button" class="btn-secondary px-3 py-1 text-sm" :disabled="busy" @click="reset(u)">Şifre sıfırlama bağlantısı</button>
            </td>
          </tr>
          <tr v-for="i in invites" :key="i.id" class="border-rule border-b last:border-b-0">
            <td class="px-4 py-3">{{ i.email }}</td>
            <td class="text-ink-soft num px-4 py-3">Davet edildi, {{ formatDateTime(i.expires_at) }} tarihine kadar geçerli</td>
            <td class="px-4 py-3 text-right">
              <button type="button" class="btn-secondary px-3 py-1 text-sm" :disabled="busy" @click="revoke(i)">Daveti iptal et</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </template>
</template>
