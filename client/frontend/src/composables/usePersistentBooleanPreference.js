import { onMounted, ref } from 'vue'
import settingsApi from '@/api/settings.js'

function parseBoolean(value, fallback) {
  if (value === true || value === 'true' || value === '1') return true
  if (value === false || value === 'false' || value === '0') return false
  return fallback
}

function readLocalValue(key, fallback) {
  try {
    return parseBoolean(localStorage.getItem(key), fallback)
  } catch {
    return fallback
  }
}

function writeLocalValue(key, value) {
  try {
    localStorage.setItem(key, String(value))
  } catch {
    // Local storage is an acceleration layer; the backend remains authoritative.
  }
}

export function usePersistentBooleanPreference(key, defaultValue = false) {
  const value = ref(readLocalValue(key, defaultValue))
  const ready = ref(false)

  async function hydrate() {
    try {
      const response = await settingsApi.getKVSetting(key)
      const persisted = String(response?.value ?? '').trim()
      if (persisted) {
        value.value = parseBoolean(persisted, value.value)
        writeLocalValue(key, value.value)
      }
    } catch {
      // Keep the immediately available local preference when the backend is offline.
    } finally {
      ready.value = true
    }
  }

  async function persist(nextValue) {
    value.value = Boolean(nextValue)
    writeLocalValue(key, value.value)
    try {
      await settingsApi.setKVSetting(key, String(value.value))
    } catch {
      // The local value still preserves the user's choice for this browser.
    }
    return value.value
  }

  onMounted(hydrate)

  return {
    value,
    ready,
    persist,
    hydrate
  }
}

export default usePersistentBooleanPreference
