<script setup>
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { LogIn, Mail } from 'lucide-vue-next'
import { appConfig, passwordLogin, requestMagicLink } from '../api.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const mode = ref('password')
const email = ref('')
const password = ref('')
const submitting = ref(false)
const sent = ref(false)
const error = ref('')

const disabled = computed(() => !appConfig.portalEnabled)
const canSubmit = computed(() => {
  if (!email.value.trim()) return false
  return mode.value === 'magic' || !!password.value
})

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

function switchMode(next) {
  mode.value = next
  error.value = ''
  sent.value = false
  password.value = ''
}

async function submit() {
  if (submitting.value || !canSubmit.value) return
  error.value = ''
  submitting.value = true
  try {
    if (mode.value === 'password') {
      await passwordLogin(email.value.trim(), password.value)
      router.push({ name: 'conversations' })
    } else {
      await requestMagicLink(email.value.trim())
      sent.value = true
    }
  } catch (err) {
    error.value = err?.response?.data?.message || t('portal.login.session')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div
    class="flex min-h-screen items-center justify-center bg-gradient-to-br from-primary/10 via-background to-background px-4 py-10 text-foreground"
  >
    <div class="w-full max-w-md">
      <div class="mb-8 text-center">
        <img v-if="appConfig.logoUrl" :src="appConfig.logoUrl" alt="" class="mx-auto mb-4 h-10 w-auto" />
        <div
          v-else
          class="mx-auto mb-4 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-lg shadow-primary/20"
        >
          <LogIn class="size-6" />
        </div>
        <h1 class="text-2xl font-semibold tracking-tight">{{ t('portal.login.title') }}</h1>
        <p class="mt-2 text-sm text-muted-foreground">
          {{ mode === 'password' ? t('portal.login.passwordSubtitle') : t('portal.login.subtitle') }}
        </p>
      </div>

      <div class="rounded-xl border border-border bg-card p-6 shadow-xl shadow-primary/5">
        <div v-if="disabled" class="rounded-lg border border-border bg-muted/40 p-4 text-center text-sm">
          {{ t('portal.login.disabled') }}
        </div>

        <div v-else-if="sent" class="text-center">
          <div class="mx-auto mb-3 flex size-11 items-center justify-center rounded-full bg-primary/10 text-primary">
            <Mail class="size-5" />
          </div>
          <p class="text-sm">{{ t('portal.login.sent') }}</p>
          <Button variant="ghost" class="mt-3" @click="sent = false">
            {{ t('portal.login.email') }}
          </Button>
        </div>

        <form v-else data-testid="portal-login-form" class="space-y-4" @submit.prevent="submit">
          <p v-if="routeError" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {{ routeError }}
          </p>
          <p v-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {{ error }}
          </p>

          <div>
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
          </div>

          <div v-if="mode === 'password'">
            <label for="portal-password" class="mb-2 block text-sm font-medium">
              {{ t('portal.login.password') }}
            </label>
            <Input
              id="portal-password"
              v-model="password"
              type="password"
              required
              autocomplete="current-password"
            />
          </div>

          <Button type="submit" data-testid="portal-login-submit" class="w-full" :disabled="submitting || !canSubmit">
            <Spinner v-if="submitting" size="sm" :absolute="false" :text="''" />
            <span v-else>
              {{ mode === 'password' ? t('portal.login.signIn') : t('portal.login.submit') }}
            </span>
          </Button>

          <Button
            v-if="mode === 'password'"
            variant="ghost"
            type="button"
            data-testid="portal-magic-toggle"
            class="w-full text-muted-foreground"
            @click="switchMode('magic')"
          >
            <Mail class="size-4" />
            {{ t('portal.login.magicLink') }}
          </Button>
          <Button
            v-else
            variant="ghost"
            type="button"
            data-testid="portal-password-toggle"
            class="w-full text-muted-foreground"
            @click="switchMode('password')"
          >
            <LogIn class="size-4" />
            {{ t('portal.login.passwordLogin') }}
          </Button>
        </form>
      </div>

      <p class="mt-6 text-center text-xs text-muted-foreground">{{ appConfig.siteName }}</p>
    </div>
  </div>
</template>
