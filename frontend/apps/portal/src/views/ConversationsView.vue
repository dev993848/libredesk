<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Badge } from '@shared-ui/components/ui/badge'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { Spinner } from '@shared-ui/components/ui/spinner'
import { appConfig, listConversations, logout } from '../api.js'
import { formatDate, formatDuration } from '../utils/format.js'

const { t, locale } = useI18n()
const router = useRouter()

const items = ref([])
const total = ref(0)
const loading = ref(true)
const error = ref('')
const status = ref('')
const query = ref('')

const filters = computed(() => [
  { value: '', label: t('portal.filters.all') },
  { value: 'open', label: t('portal.filters.open') },
  { value: 'waiting', label: t('portal.filters.waiting') },
  { value: 'resolved', label: t('portal.filters.resolved') },
])

function statusLabel(conversation) {
  const category = conversation.status?.category
  const key = `portal.status.${category}`
  const label = t(key)
  return label === key ? conversation.status?.name || '' : label
}

function badgeVariant(conversation) {
  switch (conversation.status?.category) {
    case 'open':
      return 'default'
    case 'waiting':
      return 'secondary'
    default:
      return 'outline'
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const resp = await listConversations({
      status: status.value,
      q: query.value.trim(),
      page: 1,
      per_page: 50,
    })
    items.value = resp.data.data.results || []
    total.value = resp.data.data.total || 0
  } catch (err) {
    error.value = err?.response?.data?.message || t('portal.error.load')
  } finally {
    loading.value = false
  }
}

function setStatus(value) {
  status.value = value
  load()
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
          <h1 class="truncate text-lg font-semibold">{{ t('portal.conversations.title') }}</h1>
          <p class="truncate text-sm text-muted-foreground">{{ appConfig.siteName }}</p>
        </div>
        <Button variant="ghost" size="sm" @click="signOut">{{ t('portal.logout') }}</Button>
      </div>
    </header>

    <main class="mx-auto max-w-3xl px-4 py-6">
      <div class="mb-4 flex flex-wrap gap-2">
        <Button
          v-for="filter in filters"
          :key="filter.value"
          :variant="status === filter.value ? 'default' : 'outline'"
          size="sm"
          @click="setStatus(filter.value)"
        >
          {{ filter.label }}
        </Button>
      </div>

      <Input
        v-model="query"
        type="search"
        :placeholder="t('portal.conversations.search')"
        class="mb-4"
        @keyup.enter="load"
      />

      <div v-if="loading" class="py-12">
        <Spinner size="md" :absolute="false" :text="t('portal.loading')" />
      </div>

      <p v-else-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
        {{ error }}
      </p>

      <div v-else-if="!items.length" class="rounded-lg border border-border bg-card px-4 py-12 text-center text-sm text-muted-foreground">
        {{ t('portal.conversations.empty') }}
      </div>

      <div v-else class="overflow-hidden rounded-lg border border-border bg-card">
        <RouterLink
          v-for="conversation in items"
          :key="conversation.uuid"
          :to="{ name: 'conversation', params: { uuid: conversation.uuid } }"
          class="block border-b border-border px-4 py-4 transition-colors last:border-b-0 hover:bg-accent/50"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="mb-1 flex flex-wrap items-center gap-2">
                <Badge :variant="badgeVariant(conversation)">{{ statusLabel(conversation) }}</Badge>
                <span class="text-xs text-muted-foreground">
                  {{ t('portal.reference', { number: conversation.reference_number }) }}
                </span>
              </div>
              <p class="truncate text-sm font-medium">
                {{ conversation.subject || conversation.last_message_preview || '—' }}
              </p>
              <p class="mt-1 truncate text-sm text-muted-foreground">
                {{ conversation.last_message_preview }}
              </p>
            </div>
            <div class="flex flex-shrink-0 flex-col items-end gap-1 text-right">
              <span class="text-xs text-muted-foreground">
                {{ formatDate(conversation.last_message_at || conversation.created_at, locale) }}
              </span>
              <span v-if="conversation.time_spent_seconds > 0" class="text-xs text-muted-foreground">
                {{ t('portal.time.spent', { duration: formatDuration(conversation.time_spent_seconds, t) }) }}
              </span>
              <Badge v-if="conversation.unread_count > 0" variant="success">
                {{ conversation.unread_count }}
              </Badge>
            </div>
          </div>
        </RouterLink>
      </div>

      <p v-if="total > items.length" class="mt-3 text-center text-xs text-muted-foreground">
        {{ items.length }} / {{ total }}
      </p>
    </main>
  </div>
</template>
