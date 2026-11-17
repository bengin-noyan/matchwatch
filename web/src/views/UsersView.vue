<script setup lang="ts">
// The company's logins and open invites. An invite link goes only to the invited address by e-mail, never to this
// screen, so nobody can open an account in someone else's name. A forgotten password is reset from the login page.
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
const done = ref('')

const messages: Record<string, string> = {
  INVALID_EMAIL: 'E-posta adresi geçerli değil.',
  EMAIL_TAKEN: 'Bu e-postayla zaten bir hesap var.',
  MAIL_DISABLED: 'Bu sunucuda e-posta gönderimi kapalı; davet gönderilemiyor.',
  MAIL_FAILED: 'Davet e-postası gönderilemedi. Biraz sonra tekrar dene.',
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

async function invite() {
  busy.value = true
  error.value = ''
  done.value = ''
  try {
    const r = await api.invite(email.value)
    done.value = `Davet ${r.email} adresine gönderildi; ${formatDateTime(r.expires_at)} tarihine kadar geçerli.`
    email.value = ''
    await load()
  } catch (e) {
    fail(e, 'Davet oluşturulamadı.')
  } finally {
    busy.value = false
  }
}

async function revoke(i: InviteRow) {
  busy.value = true
  error.value = ''
  try {
    await api.revokeInvite(i.id)
    done.value = `${i.email} davetini iptal ettin.`
    await load()
  } catch (e) {
    fail(e, 'Davet iptal edilemedi.')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <header class="mb-6">
    <h1 class="text-3xl font-semibold tracking-tight">Kullanıcılar</h1>
    <p class="text-ink-soft mt-1 max-w-prose">
      Firmanın hesabına giriş yapabilen kişiler. Davet ettiğin kişiye tek kullanımlık bir bağlantı e-postayla gider;
      şifresini o belirler. Şifresini unutan, giriş sayfasındaki “Şifremi unuttum” ile kendine yeni bağlantı ister.
    </p>
  </header>

  <p v-if="error" role="alert" class="text-pencil mb-4">{{ error }}</p>

  <p v-if="done" role="status" class="text-ballpoint mb-4 font-semibold">{{ done }}</p>

  <form class="bg-sheet border-rule rounded-sheet mb-6 flex flex-wrap items-end gap-3 border p-5" @submit.prevent="invite">
    <label class="min-w-0 flex-1">
      <span class="mb-1 block text-sm font-semibold">Davet edilecek kişinin e-postası</span>
      <input v-model="email" type="email" required maxlength="254" autocomplete="off" class="field" />
    </label>
    <button type="submit" class="btn-primary" :disabled="busy">Davet gönder</button>
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
            <td class="px-4 py-3" />
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
