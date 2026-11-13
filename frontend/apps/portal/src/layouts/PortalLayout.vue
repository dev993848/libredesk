<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Inbox, LogOut, UserRound } from 'lucide-vue-next'
import { Button } from '@shared-ui/components/ui/button'
import { appConfig, getMe, logout } from '../api.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const contact = ref(null)

const conversationsActive = computed(() =>
  ['conversations', 'conversation', 'new-conversation'].includes(route.name)
)
const profileActive = computed(() => route.name === 'profile')
const displayName = computed(() => {
  const name = [contact.value?.first_name, contact.value?.last_name].filter(Boolean).join(' ')
  return name || contact.value?.email || ''
})
const initials = computed(() => (contact.value?.first_name || contact.value?.email || '?').slice(0, 1).toUpperCase())

const navClass = (active) =>
  active
    ? 'bg-primary text-primary-foreground'
    : 'text-muted-foreground hover:bg-accent hover:text-foreground'

onMounted(async () => {
  try {
    const resp = await getMe()
    contact.value = resp.data.data
  } catch {
    // The router guard handles expired sessions.
  }
})

async function signOut() {
  try {
    await logout()
  } finally {
    router.push({ name: 'login' })
  }
}
</script>

<template>
  <div class="min-h-screen bg-background text-foreground lg:flex">
    <aside
      class="border-b border-border bg-card lg:sticky lg:top-0 lg:flex lg:h-screen lg:w-64 lg:flex-col lg:border-b-0 lg:border-r"
    >
      <div class="flex items-center gap-3 px-5 py-4">
        <img v-if="appConfig.logoUrl" :src="appConfig.logoUrl" alt="" class="h-8 w-auto" />
        <div class="min-w-0">
          <p class="truncate text-sm font-semibold">{{ appConfig.siteName }}</p>
          <p class="truncate text-xs text-muted-foreground">{{ t('portal.nav.supportPortal') }}</p>
        </div>
      </div>

      <nav class="flex gap-1 overflow-x-auto px-3 pb-3 lg:flex-col lg:overflow-visible lg:pb-0">
        <RouterLink
          :to="{ name: 'conversations' }"
          data-testid="portal-nav-conversations"
          class="flex items-center gap-2 whitespace-nowrap rounded-md px-3 py-2 text-sm font-medium transition-colors"
          :class="navClass(conversationsActive)"
        >
          <Inbox class="size-4" />
          {{ t('portal.nav.conversations') }}
        </RouterLink>
        <RouterLink
          :to="{ name: 'profile' }"
          data-testid="portal-nav-profile"
          class="flex items-center gap-2 whitespace-nowrap rounded-md px-3 py-2 text-sm font-medium transition-colors"
          :class="navClass(profileActive)"
        >
          <UserRound class="size-4" />
          {{ t('portal.nav.profile') }}
        </RouterLink>
      </nav>

      <div class="mt-auto hidden border-t border-border p-3 lg:block">
        <div class="mb-2 flex items-center gap-2 px-2 py-1">
          <div
            class="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary"
          >
            {{ initials }}
          </div>
          <div class="min-w-0">
            <p class="truncate text-sm font-medium">{{ displayName }}</p>
            <p class="truncate text-xs text-muted-foreground">{{ contact?.email }}</p>
          </div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          class="w-full justify-start text-muted-foreground"
          data-testid="portal-logout"
          @click="signOut"
        >
          <LogOut class="size-4" />
          {{ t('portal.logout') }}
        </Button>
      </div>
    </aside>

    <div class="min-w-0 flex-1">
      <header class="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-3 lg:hidden">
        <span class="truncate text-sm font-semibold">{{ appConfig.siteName }}</span>
        <Button variant="ghost" size="sm" @click="signOut">
          <LogOut class="size-4" />
          {{ t('portal.logout') }}
        </Button>
      </header>

      <main class="mx-auto w-full max-w-4xl px-4 py-6 lg:px-8 lg:py-8">
        <RouterView />
      </main>
    </div>
  </div>
</template>
