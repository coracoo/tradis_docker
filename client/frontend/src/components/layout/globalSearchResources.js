const RESULT_LIMIT = 5

export function text(value) {
  return String(value ?? '').trim()
}

export function includesQuery(values, query) {
  const terms = text(query).toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return false
  const haystack = values.map(text).join(' ').toLowerCase()
  return terms.every(term => haystack.includes(term))
}

export function take(items, predicate, mapper) {
  if (!Array.isArray(items)) return []
  return items.filter(predicate).slice(0, RESULT_LIMIT).map(mapper)
}

export function searchLocalManagedResources(dataset = {}, query = '') {
  const navigation = take(
    dataset.navigationApps,
    item => includesQuery([item?.title, item?.category, item?.lan_url, item?.wan_url], query),
    item => ({
      id: `navigation-${item.id}`,
      type: 'navigation',
      name: text(item.title) || '未命名导航',
      description: text(item.category) || text(item.lan_url) || '导航应用',
      route: { path: '/navigation' }
    })
  )

  const scheduledTasks = take(
    dataset.scheduledTasks,
    item => includesQuery([item?.Name, item?.name, item?.TaskType, item?.taskType, item?.CronExpr, item?.cronExpr], query),
    item => ({
      id: `scheduled-task-${item.ID ?? item.id}`,
      type: 'scheduled-task',
      name: text(item.Name || item.name) || '未命名定时任务',
      description: text(item.CronExpr || item.cronExpr) || text(item.TaskType || item.taskType) || '定时任务',
      route: { path: '/scheduled-tasks' }
    })
  )

  return [...navigation, ...scheduledTasks]
}

export function searchEditionManagedResources() {
  return []
}
