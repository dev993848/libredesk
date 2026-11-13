<script setup>
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@shared-ui/components/ui/button'
import { Input } from '@shared-ui/components/ui/input'
import api from '@main/api'

const { t } = useI18n()

const loading = ref(true)
const saving = ref(false)
const saved = ref(false)
const error = ref('')
const inboxes = ref([])

const form = ref({
  enabled: true,
  defaultInboxID: 0,
  rootURL: '',
  jwtEnabled: false,
  jwtSecret: '',
  sessionLifetime: '720h',
  magicLinkTTL: '15m',
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [settingsResp, inboxResp] = await Promise.all([
      api.getPortalSettings(),
      api.getInboxes(),
    ])
    const s = settingsResp.data.data || {}
    form.value = {
      enabled: s['portal.enabled'] !== false,
      defaultInboxID: s['portal.default_inbox_id'] || 0,
      rootURL: s['portal.root_url'] || '',
      jwtEnabled: !!s['portal.jwt_enabled'],
      jwtSecret: s['portal.jwt_secret'] || '',
      sessionLifetime: s['portal.session_lifetime'] || '720h',
      magicLinkTTL: s['portal.magic_link_ttl'] || '15m',
    }
    inboxes.value = inboxResp.data.data || []
  } catch (err) {
    error.value = err?.response?.data?.message || t('globals.messages.somethingWentWrong')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  saved.value = false
  error.value = ''
  try {
    await api.updatePortalSettings({
      'portal.enabled': form.value.enabled,
      'portal.default_inbox_id': Number(form.value.defaultInboxID) || 0,
      'portal.root_url': form.value.rootURL,
      'portal.jwt_enabled': form.value.jwtEnabled,
      'portal.jwt_secret': form.value.jwtSecret,
      'portal.session_lifetime': form.value.sessionLifetime,
      'portal.magic_link_ttl': form.value.magicLinkTTL,
    })
    saved.value = true
  } catch (err) {
    error.value = err?.response?.data?.message || t('globals.messages.somethingWentWrong')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="box p-5">
    <h2 class="ui-heading mb-4">{{ t('admin.portal.title') }}</h2>

    <p v-if="loading" class="text-sm text-muted-foreground">{{ t('globals.terms.loading') }}</p>

    <form v-else class="space-y-4" @submit.prevent="save">
      <p v-if="error" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{{ error }}</p>
      <p v-if="saved" class="rounded-md bg-success/10 px-3 py-2 text-sm text-success">{{ t('admin.portal.saved') }}</p>

      <label class="flex items-center gap-2 text-sm">
        <input v-model="form.enabled" type="checkbox" class="size-4" />
        {{ t('admin.portal.enabled') }}
      </label>
      <p class="text-xs text-muted-foreground">{{ t('admin.portal.enabledHint') }}</p>

      <div>
        <label class="mb-2 block text-sm font-medium">{{ t('admin.portal.defaultInbox') }}</label>
        <select
          v-model="form.defaultInboxID"
          class="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm"
        >
          <option :value="0">{{ t('admin.portal.noInbox') }}</option>
          <option v-for="inbox in inboxes" :key="inbox.id" :value="inbox.id">
            {{ inbox.name }} ({{ inbox.channel }})
          </option>
        </select>
        <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.portal.defaultInboxHint') }}</p>
      </div>

      <div>
        <label class="mb-2 block text-sm font-medium">{{ t('admin.portal.rootURL') }}</label>
        <Input v-model="form.rootURL" placeholder="https://support.example.com" />
        <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.portal.rootURLHint') }}</p>
      </div>

      <label class="flex items-center gap-2 text-sm">
        <input v-model="form.jwtEnabled" type="checkbox" class="size-4" />
        {{ t('admin.portal.jwtEnabled') }}
      </label>

      <div v-if="form.jwtEnabled">
        <label class="mb-2 block text-sm font-medium">{{ t('admin.portal.jwtSecret') }}</label>
        <Input v-model="form.jwtSecret" type="password" autocomplete="new-password" />
        <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.portal.jwtSecretHint') }}</p>
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div>
          <label class="mb-2 block text-sm font-medium">{{ t('admin.portal.sessionLifetime') }}</label>
          <Input v-model="form.sessionLifetime" placeholder="720h" />
        </div>
        <div>
          <label class="mb-2 block text-sm font-medium">{{ t('admin.portal.magicLinkTTL') }}</label>
          <Input v-model="form.magicLinkTTL" placeholder="15m" />
        </div>
      </div>

      <div class="flex justify-end">
        <Button type="submit" :disabled="saving">
          {{ saving ? t('globals.messages.saving') : t('globals.messages.saveChanges') }}
        </Button>
      </div>
    </form>
  </div>
</template>
