export function matchesBooleanFilter(actual, filter) {
  if (filter === 'yes') return Boolean(actual)
  if (filter === 'no') return !actual
  return true
}
