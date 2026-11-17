import { defineStore } from 'pinia'
import { api, type Me } from './api'

export const useSession = defineStore('session', {
  state: () => ({ me: null as Me | null, checked: false }),
  actions: {
    async load() {
      try {
        this.me = await api.me()
      } catch {
        this.me = null
      }
      this.checked = true
    },
    async login(email: string, password: string) {
      this.me = await api.login(email, password)
      this.checked = true
    },
    async signup(b: Parameters<typeof api.signup>[0]) {
      this.me = await api.signup(b)
      this.checked = true
    },
    async redeem(token: string, password: string) {
      this.me = await api.redeem(token, password)
      this.checked = true
    },
    async logout() {
      await api.logout().catch(() => {})
      this.me = null
    },
    expired() {
      this.me = null
    },
  },
})
