export function formatDuration(seconds, t) {
  const total = Math.max(0, Math.round(Number(seconds) || 0))
  if (total === 0) {
    return `0 ${t('portal.time.minute')}`
  }

  const totalMinutes = Math.round(total / 60)
  if (totalMinutes === 0) {
    return t('portal.time.lessThanMinute')
  }

  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60

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
