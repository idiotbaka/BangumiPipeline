import { currentAPIBaseURL } from './api'

const ignoredVersionStorageKey = 'bp.mobile.ignoredAppVersion'
const numericVersionPattern = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/

export function isNewerAppVersion(candidate: string, current: string) {
  const candidateParts = parseVersion(candidate)
  const currentParts = parseVersion(current)
  if (!candidateParts || !currentParts) {
    return false
  }
  for (let index = 0; index < candidateParts.length; index += 1) {
    if (candidateParts[index] !== currentParts[index]) {
      return candidateParts[index] > currentParts[index]
    }
  }
  return false
}

export function ignoredAppVersion() {
  try {
    return localStorage.getItem(ignoredVersionStorageKey) ?? ''
  } catch {
    return ''
  }
}

export function ignoreAppVersion(version: string) {
  try {
    localStorage.setItem(ignoredVersionStorageKey, version)
  } catch {
    // Ignoring an update is best effort when WebView storage is unavailable.
  }
}

export function appDownloadURL() {
  return new URL('app/download', currentAPIBaseURL()).toString()
}

function parseVersion(value: string) {
  const normalized = value.trim()
  if (!numericVersionPattern.test(normalized)) {
    return null
  }
  return normalized.split('.').map((part) => Number(part))
}
