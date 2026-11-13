<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { ShieldCheck } from 'lucide-vue-next'
import { getMe, setPassword } from '../api.js'

const { t } = useI18n()

const contact = ref(null)
const loading = ref(true)
const error = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const passwordSaving = ref(false)
const passwordError = ref('')
const passwordSaved = ref(false)

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

async function savePassword() {
  passwordError.value = ''
  passwordSaved.value = false
  if (newPassword.value !== confirmPassword.value) {
    passwordError.value = t('portal.profile.passwordsMismatch')
    return
  }
  if (!newPassword.value) {
    passwordError.value = t('auth.passwordRequired')
    return
  }
  passwordSaving.value = true
  try {
    await setPassword(newPassword.value)
    newPassword.value = ''
    confirmPassword.value = ''
    passwordSaved.value = true
  } catch (err) {
    passwordError.value = err?.response?.data?.message || t('globals.messages.somethingWentWrong')
  } finally {
    passwordSaving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <h1 class="mb-6 text-2xl font-semibold tracking-tight">{{ t('portal.profile.title') }}</h1>

    <div v-if="loading" class="py-12">
      <Spinner size="md" :absolute="false" :text="t('portal.loading')" />
    </div>

    <p v-else-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
      {{ error }}
    </p>

    <div v-else-if="contact" class="space-y-6">
      <div class="rounded-xl border border-border bg-card p-6">
        <div class="flex items-center gap-4">
          <img
            v-if="contact.avatar_url"
            :src="contact.avatar_url"
            alt=""
            class="size-14 rounded-full object-cover"
          />
          <div
            v-else
            class="flex size-14 items-center justify-center rounded-full bg-primary/10 text-lg font-semibold text-primary"
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
      </div>

      <div class="rounded-xl border border-border bg-card p-6">
        <div class="flex items-center gap-2">
          <ShieldCheck class="size-5 text-primary" />
          <h2 class="text-base font-semibold">{{ t('portal.profile.passwordTitle') }}</h2>
        </div>
        <p class="mt-1 text-sm text-muted-foreground">{{ t('portal.profile.passwordHint') }}</p>

        <form class="mt-4 space-y-3" @submit.prevent="savePassword">
          <div>
            <label class="mb-2 block text-sm font-medium" for="portal-new-password">
              {{ t('portal.profile.newPassword') }}
            </label>
            <Input
              id="portal-new-password"
              v-model="newPassword"
              data-testid="portal-new-password"
              type="password"
              autocomplete="new-password"
            />
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium" for="portal-confirm-password">
              {{ t('portal.profile.confirmPassword') }}
            </label>
            <Input
              id="portal-confirm-password"
              v-model="confirmPassword"
              data-testid="portal-confirm-password"
              type="password"
              autocomplete="new-password"
            />
          </div>

          <p v-if="passwordError" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {{ passwordError }}
          </p>
          <p v-if="passwordSaved" data-testid="portal-password-saved" class="rounded-md bg-success/10 px-3 py-2 text-sm text-success">
            {{ t('portal.profile.passwordSaved') }}
          </p>

          <Button type="submit" data-testid="portal-save-password" :disabled="passwordSaving">
            {{ passwordSaving ? t('portal.loading') : t('portal.profile.savePassword') }}
          </Button>
        </form>
      </div>
    </div>
  </div>
</template>
