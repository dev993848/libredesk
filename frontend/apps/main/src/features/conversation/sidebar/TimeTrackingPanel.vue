<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import { formatDuration } from '@shared-ui/utils/datetime.js'
import { handleHTTPError } from '@shared-ui/utils/http.js'
import { useConversationStore } from '@main/stores/conversation'
import api from '@main/api'

const { t } = useI18n()
const conversationStore = useConversationStore()

const entries = ref([])
const totalSeconds = ref(0)
const billableSeconds = ref(0)
const active = ref(null)
const hasActive = ref(false)
const loading = ref(true)
const error = ref('')
const minutes = ref('')
const note = ref('')
const billable = ref(true)
const saving = ref(false)
const editingId = ref(null)
const editMinutes = ref('')
const editNote = ref('')
const editBillable = ref(true)

const uuid = computed(() => conversationStore.current?.uuid)
const activeHere = computed(() => hasActive.value && active.value?.conversation_id === conversationStore.current?.id)

async function load() {
  if (!uuid.value) return
  loading.value = true
  error.value = ''
  try {
    const resp = await api.get(`/api/v1/conversations/${uuid.value}/time-entries`)
    const data = resp.data.data
    entries.value = data.entries || []
    totalSeconds.value = data.total_seconds || 0
    billableSeconds.value = data.billable_seconds || 0
    active.value = data.active || null
    hasActive.value = !!data.has_active
  } catch (err) {
    error.value = err?.response?.data?.message || t('globals.messages.somethingWentWrong')
  } finally {
    loading.value = false
  }
}

async function startTimer() {
  try {
    await api.post(`/api/v1/conversations/${uuid.value}/timer/start`)
    await load()
  } catch (err) {
    handleHTTPError(err, t)
  }
}

async function stopTimer() {
  try {
    await api.post(`/api/v1/conversations/${uuid.value}/timer/stop`)
    await load()
  } catch (err) {
    handleHTTPError(err, t)
  }
}

async function addManual() {
  const mins = Number(minutes.value)
  if (!mins || mins <= 0) return
  saving.value = true
  try {
    await api.post(`/api/v1/conversations/${uuid.value}/time-entries`, {
      duration_seconds: Math.round(mins * 60),
      note: note.value.trim(),
      billable: billable.value,
    })
    minutes.value = ''
    note.value = ''
    await load()
  } catch (err) {
    handleHTTPError(err, t)
  } finally {
    saving.value = false
  }
}

function startEdit(entry) {
  editingId.value = entry.id
  editMinutes.value = String(Math.max(1, Math.round((entry.duration_seconds || 0) / 60)))
  editNote.value = entry.note || ''
  editBillable.value = !!entry.billable
}

function cancelEdit() {
  editingId.value = null
}

async function saveEdit(entry) {
  const mins = Number(editMinutes.value)
  if (!mins || mins <= 0) return
  saving.value = true
  try {
    await api.put(`/api/v1/time-entries/${entry.id}`, {
      duration_seconds: Math.round(mins * 60),
      note: editNote.value.trim(),
      billable: editBillable.value,
    })
    editingId.value = null
    await load()
  } catch (err) {
    handleHTTPError(err, t)
  } finally {
    saving.value = false
  }
}

async function removeEntry(entry) {
  if (!window.confirm(t('timeTracking.confirmDelete'))) return
  try {
    await api.delete(`/api/v1/time-entries/${entry.id}`)
    await load()
  } catch (err) {
    handleHTTPError(err, t)
  }
}

onMounted(load)

watch(uuid, (value, previous) => {
  if (value && value !== previous) {
    editingId.value = null
    load()
  }
})
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-2">
      <div>
        <p class="text-sm font-medium">{{ t('timeTracking.total') }}</p>
        <p class="text-lg font-semibold">{{ formatDuration(totalSeconds, false) }}</p>
        <p v-if="billableSeconds" class="text-xs text-muted-foreground">
          {{ t('timeTracking.billable') }}: {{ formatDuration(billableSeconds, false) }}
        </p>
      </div>
      <Button
        v-if="activeHere"
        variant="destructive"
        size="sm"
        @click="stopTimer"
      >
        {{ t('timeTracking.stop') }}
      </Button>
      <Button
        v-else
        size="sm"
        :disabled="hasActive && !activeHere"
        :title="hasActive && !activeHere ? t('timeTracking.otherTimer') : ''"
        @click="startTimer"
      >
        {{ t('timeTracking.start') }}
      </Button>
    </div>

    <p v-if="hasActive && !activeHere" class="text-xs text-muted-foreground">
      {{ t('timeTracking.otherTimer') }}
    </p>

    <p v-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-xs text-destructive">{{ error }}</p>

    <div v-if="loading" class="text-sm text-muted-foreground">{{ t('globals.terms.loading') }}</div>

    <div v-else class="space-y-2">
      <div
        v-for="entry in entries"
        :key="entry.id"
        class="rounded-md border border-border px-3 py-2 text-sm"
      >
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0 flex-1">
            <template v-if="editingId === entry.id">
              <div class="space-y-2">
                <div class="flex gap-2">
                  <Input v-model="editMinutes" type="number" min="1" step="1" class="w-24" :placeholder="t('timeTracking.minutes')" />
                  <Input v-model="editNote" class="flex-1" :placeholder="t('timeTracking.note')" />
                </div>
                <label class="flex items-center gap-2 text-xs text-muted-foreground">
                  <input v-model="editBillable" type="checkbox" class="size-3.5" />
                  {{ t('timeTracking.billableFlag') }}
                </label>
                <div class="flex gap-2">
                  <Button size="xs" :disabled="saving || !editMinutes" @click="saveEdit(entry)">
                    {{ t('globals.messages.save') }}
                  </Button>
                  <Button variant="ghost" size="xs" @click="cancelEdit">
                    {{ t('globals.messages.cancel') }}
                  </Button>
                </div>
              </div>
            </template>
            <template v-else>
              <p class="font-medium">
                {{ formatDuration(entry.duration_seconds, false) }}
                <span v-if="!entry.ended_at" class="text-xs text-muted-foreground">({{ t('timeTracking.running') }})</span>
              </p>
              <p class="truncate text-xs text-muted-foreground">
                {{ entry.user_name || '—' }} · {{ new Date(entry.started_at).toLocaleString() }}
              </p>
              <p v-if="entry.note" class="mt-1 text-xs text-muted-foreground">{{ entry.note }}</p>
            </template>
          </div>
          <div v-if="editingId !== entry.id" class="flex gap-1">
            <Button v-if="entry.ended_at" variant="ghost" size="xs" @click="startEdit(entry)">
              {{ t('timeTracking.edit') }}
            </Button>
            <Button variant="ghost" size="xs" @click="removeEntry(entry)">
              {{ t('globals.messages.delete') }}
            </Button>
          </div>
        </div>
      </div>
      <p v-if="!entries.length" class="text-sm text-muted-foreground">{{ t('timeTracking.empty') }}</p>
    </div>

    <div class="space-y-2 border-t border-border pt-3">
      <p class="text-sm font-medium">{{ t('timeTracking.addManual') }}</p>
      <div class="flex gap-2">
        <Input v-model="minutes" type="number" min="1" step="1" :placeholder="t('timeTracking.minutes')" class="w-28" />
        <Input v-model="note" :placeholder="t('timeTracking.note')" class="flex-1" />
      </div>
      <label class="flex items-center gap-2 text-xs text-muted-foreground">
        <input v-model="billable" type="checkbox" class="size-3.5" />
        {{ t('timeTracking.billableFlag') }}
      </label>
      <Button size="sm" :disabled="saving || !minutes" @click="addManual">
        {{ t('globals.messages.add') }}
      </Button>
    </div>
  </div>
</template>
