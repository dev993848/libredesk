<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { appConfig, getMe, logout } from '../api.js'

const { t } = useI18n()
const router = useRouter()

const contact = ref(null)
const loading = ref(true)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const resp = await getMe()
    contact.value = resp.data.data
  } catch (err) {
    error.value = err?.response?.data?.message || t('portal.error.load')
  } finally {
    loading.value = false
  }
}

async function signOut() {
  try {
    await logout()
  } finally {
    router.push({ name: 'login' })
  }
}

onMounted(load)
</script>

<template>
  <div class="min-h-screen bg-background text-foreground">
    <header class="border-b border-border bg-card">
      <div class="mx-auto flex max-w-3xl items-center justify-between gap-4 px-4 py-4">
        <div class="min-w-0">
          <RouterLink :to="{ name: 'conversations' }" class="text-sm text-muted-foreground hover:text-foreground">
            {{ t('portal.back') }}
          </RouterLink>
          <p class="mt-1 truncate text-sm font-medium">{{ appConfig.siteName }}</p>
        </div>
      </div>
    </header>

    <main class="mx-auto max-w-3xl px-4 py-6">
      <h1 class="mb-4 text-xl font-semibold tracking-tight">{{ t('portal.profile.title') }}</h1>

      <div v-if="loading" class="py-12">
        <Spinner size="md" :absolute="false" :text="t('portal.loading')" />
      </div>

      <p v-else-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
        {{ error }}
      </p>

      <div v-else-if="contact" class="rounded-lg border border-border bg-card p-6">
        <div class="flex items-center gap-4">
          <img
            v-if="contact.avatar_url"
            :src="contact.avatar_url"
            alt=""
            class="size-14 rounded-full object-cover"
          />
          <div
            v-else
            class="flex size-14 items-center justify-center rounded-full bg-muted text-lg font-semibold"
          >
            {{ (contact.first_name || contact.email || '?').slice(0, 1).toUpperCase() }}
          </div>
          <div class="min-w-0">
            <p class="truncate text-lg font-semibold">{{ contact.first_name }} {{ contact.last_name }}</p>
            <p data-testid="portal-profile-email" class="truncate text-sm text-muted-foreground">
              {{ contact.email }}
            </p>
          </div>
        </div>

        <div class="mt-6 border-t border-border pt-4">
          <Button variant="outline" size="sm" @click="signOut">{{ t('portal.logout') }}</Button>
        </div>
      </div>
    </main>
  </div>
</template>
