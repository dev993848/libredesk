<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { appConfig, createConversation, uploadAttachment } from '../api.js'

const { t } = useI18n()
const router = useRouter()

const subject = ref('')
const message = ref('')
const files = ref([])
const submitting = ref(false)
const error = ref('')

function onFiles(event) {
  files.value = Array.from(event.target.files || [])
}

async function submit() {
  if (submitting.value) return
  error.value = ''
  submitting.value = true
  try {
    const resp = await createConversation({
      subject: subject.value.trim(),
      message: message.value.trim(),
    })
    const uuid = resp.data.data.uuid
    for (const file of files.value) {
      await uploadAttachment(uuid, file)
    }
    router.push({ name: 'conversation', params: { uuid } })
  } catch (err) {
    error.value = err?.response?.data?.message || t('portal.new.error')
  } finally {
    submitting.value = false
  }
}
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
      <h1 class="mb-4 text-xl font-semibold tracking-tight">{{ t('portal.new.title') }}</h1>

      <form class="rounded-lg border border-border bg-card p-4" @submit.prevent="submit">
        <p v-if="error" class="mb-4 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {{ error }}
        </p>

        <label for="portal-subject" class="mb-2 block text-sm font-medium">{{ t('portal.new.subject') }}</label>
        <Input id="portal-subject" data-testid="portal-subject" v-model="subject" required maxlength="255" />

        <label for="portal-message" class="mb-2 mt-4 block text-sm font-medium">{{ t('portal.new.message') }}</label>
        <textarea
          id="portal-message"
          data-testid="portal-message"
          v-model="message"
          required
          rows="8"
          class="flex min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        ></textarea>

        <div class="mt-4 flex justify-end">
          <Button type="submit" data-testid="portal-new-submit" :disabled="submitting || !subject.trim() || !message.trim()">
            {{ submitting ? t('portal.loading') : t('portal.new.submit') }}
          </Button>
        </div>

        <div class="mt-3">
          <label class="text-sm text-muted-foreground" for="portal-files">{{ t('portal.attach') }}</label>
          <input id="portal-files" type="file" multiple class="mt-1 block w-full text-sm" @change="onFiles" />
        </div>
      </form>
    </main>
  </div>
</template>
