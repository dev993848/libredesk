export function formatDuration(seconds, t) {
  const total = Math.max(0, Math.round(Number(seconds) || 0))
  const hours = Math.floor(total / 3600)
  const minutes = Math.round((total % 3600) / 60)

  if (hours > 0 && minutes > 0) {
    return `${hours} ${t('portal.time.hour')} ${minutes} ${t('portal.time.minute')}`
  }
  if (hours > 0) {
    return `${hours} ${t('portal.time.hour')}`
  }
  return `${minutes} ${t('portal.time.minute')}`
}

export function formatDate(value, locale) {
  if (!value) return ''
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}
