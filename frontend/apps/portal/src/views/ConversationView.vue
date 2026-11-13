<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { sanitize } from 'lettersanitizer'
import { Badge } from '@shared-ui/components/ui/badge'
import { Button } from '@shared-ui/components/ui/button'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { appConfig, getConversation, markSeen, replyToConversation, uploadAttachment } from '../api.js'
import { formatDate, formatDuration } from '../utils/format.js'

const { t, locale } = useI18n()
const route = useRoute()

const conversation = ref(null)
const loading = ref(true)
const error = ref('')
const replyText = ref('')
const sending = ref(false)
const replyError = ref('')
const uploading = ref(false)

const statusLabel = computed(() => {
  const category = conversation.value?.status?.category
  if (!category) return ''
  const key = `portal.status.${category}`
  const label = t(key)
  return label === key ? conversation.value?.status?.name || '' : label
})

const badgeVariant = computed(() => {
  switch (conversation.value?.status?.category) {
    case 'open':
      return 'default'
    case 'waiting':
      return 'secondary'
    default:
      return 'outline'
  }
})

function renderedContent(message) {
  if (!message?.content) return ''
  if (message.content_type === 'html') {
    return sanitize(message.content, '', { noWrapper: true })
  }
  return ''
}

function isContact(message) {
  return message.sender_type === 'contact'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const resp = await getConversation(route.params.uuid)
    conversation.value = resp.data.data
    markSeen(route.params.uuid).catch(() => {})
  } catch (err) {
    error.value = err?.response?.data?.message || t('portal.error.load')
  } finally {
    loading.value = false
  }
}

async function sendReply() {
  const text = replyText.value.trim()
  if (!text || sending.value) return
  sending.value = true
  replyError.value = ''
  try {
    await replyToConversation(route.params.uuid, text)
    replyText.value = ''
    await load()
  } catch (err) {
    replyError.value = err?.response?.data?.message || t('portal.reply.error')
  } finally {
    sending.value = false
  }
}

async function onFiles(event) {
  const files = Array.from(event.target.files || [])
  if (!files.length) return
  uploading.value = true
  replyError.value = ''
  try {
    for (const file of files) {
      await uploadAttachment(route.params.uuid, file)
    }
    event.target.value = ''
    await load()
  } catch (err) {
    replyError.value = err?.response?.data?.message || t('portal.reply.error')
  } finally {
    uploading.value = false
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
        <Button variant="ghost" size="sm" @click="$router.push({ name: 'conversations' })">
          {{ t('portal.conversations.title') }}
        </Button>
      </div>
    </header>

    <main class="mx-auto max-w-3xl px-4 py-6">
      <div v-if="loading" class="py-12">
        <Spinner size="md" :absolute="false" :text="t('portal.loading')" />
      </div>

      <p v-else-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
        {{ error }}
      </p>

      <template v-else-if="conversation">
        <div class="mb-6">
          <div class="mb-2 flex flex-wrap items-center gap-2">
            <Badge :variant="badgeVariant">{{ statusLabel }}</Badge>
            <span class="text-xs text-muted-foreground">
              {{ t('portal.reference', { number: conversation.reference_number }) }}
            </span>
          </div>
          <h1 class="text-xl font-semibold tracking-tight">{{ conversation.subject || '—' }}</h1>
          <p class="mt-1 text-sm text-muted-foreground">
            {{ conversation.inbox_name }} · {{ formatDate(conversation.created_at, locale) }}
          </p>
          <p v-if="conversation.time_spent_seconds > 0" class="mt-1 text-sm text-muted-foreground">
            {{ t('portal.time.spent', { duration: formatDuration(conversation.time_spent_seconds, t) }) }}
          </p>
        </div>

        <div v-if="!conversation.messages.length" class="rounded-lg border border-border bg-card px-4 py-12 text-center text-sm text-muted-foreground">
          {{ t('portal.noMessages') }}
        </div>

        <div v-else class="space-y-4">
          <div
            v-for="message in conversation.messages"
            :key="message.uuid"
            class="flex"
            :class="isContact(message) ? 'justify-end' : 'justify-start'"
          >
            <div
              class="max-w-[85%] rounded-lg border px-4 py-3"
              :class="isContact(message) ? 'bg-primary text-primary-foreground border-primary' : 'bg-card border-border'"
            >
              <div class="mb-1 flex items-center gap-2 text-xs opacity-80">
                <span>{{ isContact(message) ? t('portal.message.you') : message.author_name }}</span>
                <span>·</span>
                <span>{{ formatDate(message.created_at, locale) }}</span>
              </div>

              <div v-if="message.content_type === 'html'" class="portal-message" v-html="renderedContent(message)"></div>
              <p v-else class="whitespace-pre-wrap break-words text-sm">{{ message.content }}</p>

              <div v-if="message.attachments.length" class="mt-3 border-t border-current/20 pt-2">
                <p class="mb-1 text-xs font-medium opacity-80">{{ t('portal.attachments') }}</p>
                <ul class="space-y-1">
                  <li v-for="attachment in message.attachments" :key="attachment.uuid">
                    <a
                      :href="attachment.url"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="break-all text-sm underline underline-offset-2"
                    >
                      {{ attachment.name }}
                    </a>
                  </li>
                </ul>
              </div>
            </div>
          </div>
        </div>

        <form class="mt-6 rounded-lg border border-border bg-card p-4" @submit.prevent="sendReply">
          <p v-if="replyError" class="mb-3 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {{ replyError }}
          </p>
          <textarea
            v-model="replyText"
            rows="4"
            :placeholder="t('portal.reply.placeholder')"
            class="flex min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          ></textarea>
          <div class="mt-3 flex items-center justify-between gap-3">
            <label class="cursor-pointer text-sm text-muted-foreground">
              {{ uploading ? t('portal.loading') : t('portal.attach') }}
              <input type="file" multiple class="hidden" :disabled="uploading" @change="onFiles" />
            </label>
            <Button type="submit" :disabled="sending || !replyText.trim()">
              {{ sending ? t('portal.loading') : t('portal.reply.submit') }}
            </Button>
          </div>
        </form>
      </template>
    </main>
  </div>
</template>

<style scoped>
.portal-message :deep(p) {
  margin: 0 0 0.5rem;
  overflow-wrap: anywhere;
}
.portal-message :deep(p:last-child) {
  margin-bottom: 0;
}
.portal-message :deep(a) {
  text-decoration: underline;
  overflow-wrap: anywhere;
}
.portal-message :deep(img) {
  max-width: 100%;
  height: auto;
}
</style>
