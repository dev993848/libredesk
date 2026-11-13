<script setup>
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { appConfig, requestMagicLink } from '../api.js'

const { t } = useI18n()
const route = useRoute()

const email = ref('')
const submitting = ref(false)
const sent = ref(false)
const error = ref('')

const disabled = computed(() => !appConfig.portalEnabled)

const routeError = computed(() => {
  switch (route.query.error) {
    case 'invalid':
      return t('portal.login.invalid')
    case 'session':
      return t('portal.login.session')
    case 'disabled':
      return t('portal.login.disabled')
    default:
      return ''
  }
})

async function submit() {
  if (submitting.value) return
  error.value = ''
  submitting.value = true
  try {
    await requestMagicLink(email.value.trim())
    sent.value = true
  } catch (err) {
    error.value = err?.response?.data?.message || t('portal.login.session')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="min-h-screen bg-background text-foreground flex items-center justify-center px-4 py-10">
    <div class="w-full max-w-md">
      <div class="mb-8 text-center">
        <img v-if="appConfig.logoUrl" :src="appConfig.logoUrl" alt="" class="mx-auto mb-4 h-10 w-auto" />
        <h1 class="text-2xl font-semibold tracking-tight">{{ t('portal.login.title') }}</h1>
        <p class="mt-2 text-sm text-muted-foreground">{{ t('portal.login.subtitle') }}</p>
      </div>

      <div v-if="disabled" class="rounded-lg border border-border bg-card p-6 text-center text-sm">
        {{ t('portal.login.disabled') }}
      </div>

      <div v-else-if="sent" class="rounded-lg border border-border bg-card p-6 text-center">
        <p class="text-sm">{{ t('portal.login.sent') }}</p>
        <Button variant="link" class="mt-2" @click="sent = false">
          {{ t('portal.login.email') }}
        </Button>
      </div>

      <form v-else class="rounded-lg border border-border bg-card p-6 shadow-sm" @submit.prevent="submit">
        <p v-if="routeError" class="mb-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {{ routeError }}
        </p>
        <p v-if="error" class="mb-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {{ error }}
        </p>

        <label for="portal-email" class="mb-2 block text-sm font-medium">
          {{ t('portal.login.email') }}
        </label>
        <Input
          id="portal-email"
          v-model="email"
          type="email"
          required
          autocomplete="email"
          placeholder="you@example.com"
        />

        <Button type="submit" class="mt-4 w-full" :disabled="submitting || !email.trim()">
          <Spinner v-if="submitting" size="sm" :absolute="false" :text="''" />
          <span v-else>{{ t('portal.login.submit') }}</span>
        </Button>
      </form>
    </div>
  </div>
</template>
