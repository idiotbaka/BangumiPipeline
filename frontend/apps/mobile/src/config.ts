const defaultAPIBaseURL = 'https://baka.vip/'
const storageKey = 'bp.mobile.apiBaseUrl'

export interface AppConfig {
  apiBaseUrl: string
}

interface AppConfigFile {
  apiBaseUrl?: string
}

export async function loadAppConfig(): Promise<AppConfig> {
  const fileConfig = await loadConfigFile()
  const storedBaseURL = localStorage.getItem(storageKey)
  return {
    apiBaseUrl: normalizeAPIBaseURL(storedBaseURL || fileConfig.apiBaseUrl || defaultAPIBaseURL),
  }
}

export function saveAPIBaseURL(value: string) {
  localStorage.setItem(storageKey, normalizeAPIBaseURL(value))
}

export function normalizeAPIBaseURL(value: string): string {
  return parseAPIBaseURL(value) ?? defaultAPIBaseURL
}

export function parseAPIBaseURL(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed) {
    return null
  }
  if (/^[a-z][a-z\d+.-]*:\/\//i.test(trimmed) && !/^https?:\/\//i.test(trimmed)) {
    return null
  }
  const withProtocol = /^https?:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`
  try {
    const url = new URL(withProtocol)
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname || url.username || url.password) {
      return null
    }
    url.pathname = url.pathname.replace(/\/+$/, '')
    url.search = ''
    url.hash = ''
    return `${url.toString().replace(/\/+$/, '')}/`
  } catch {
    return null
  }
}

async function loadConfigFile(): Promise<AppConfigFile> {
  try {
    const response = await fetch('/app-config.json', { cache: 'no-store' })
    if (!response.ok) {
      return {}
    }
    return (await response.json()) as AppConfigFile
  } catch {
    return {}
  }
}
