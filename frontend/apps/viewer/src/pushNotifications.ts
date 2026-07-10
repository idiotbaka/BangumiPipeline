import { api, type ViewerPushSubscriptionInput } from './api'

export type PushNotificationStatus = 'enabled' | 'denied' | 'unsupported' | 'unavailable'

const initialPromptStorageKey = 'bp-viewer-push-prompted'

export async function initializePushNotifications(): Promise<PushNotificationStatus> {
  if (!pushSupported()) return 'unsupported'
  if (Notification.permission === 'granted') return syncPushSubscription()
  if (Notification.permission !== 'default' || localStorage.getItem(initialPromptStorageKey) === '1') {
    return Notification.permission === 'denied' ? 'denied' : 'unavailable'
  }
  localStorage.setItem(initialPromptStorageKey, '1')
  return requestPermissionAndSync()
}

// 此函数会在点击“追番”的同步事件中立即调用，保证浏览器将权限弹窗视为用户主动操作。
export function requestPushNotificationsForFollow(): Promise<PushNotificationStatus> {
  if (!pushSupported()) return Promise.resolve('unsupported')
  if (Notification.permission === 'granted') return syncPushSubscription()
  if (Notification.permission !== 'default') return Promise.resolve('denied')
  return requestPermissionAndSync()
}

export async function removePushNotifications(): Promise<void> {
  if (!pushSupported()) return
  try {
    const registration = await navigator.serviceWorker.getRegistration('/')
    const subscription = await registration?.pushManager.getSubscription()
    if (!subscription) return
    await api.removePushSubscription(subscription.endpoint)
    await subscription.unsubscribe()
  } catch {
    // 退出登录不能被推送清理失败阻塞；服务端的失效订阅也会在后续投递时移除。
  }
}

async function requestPermissionAndSync(): Promise<PushNotificationStatus> {
  try {
    const permission = await Notification.requestPermission()
    if (permission !== 'granted') return 'denied'
    return syncPushSubscription()
  } catch {
    return 'unavailable'
  }
}

async function syncPushSubscription(): Promise<PushNotificationStatus> {
  try {
    const [registration, configResult] = await Promise.all([
      registerPushServiceWorker(),
      api.pushConfig(),
    ])
    if (!registration || !configResult.config.supported || !configResult.config.publicKey) return 'unavailable'
    let subscription = await registration.pushManager.getSubscription()
    if (!subscription) {
      subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(configResult.config.publicKey),
      })
    }
    const payload = subscription.toJSON()
    const input = toSubscriptionInput(payload)
    if (!input) return 'unavailable'
    await api.upsertPushSubscription(input)
    return 'enabled'
  } catch {
    return 'unavailable'
  }
}

function pushSupported() {
  return window.isSecureContext && 'Notification' in window && 'serviceWorker' in navigator && 'PushManager' in window
}

function registerPushServiceWorker() {
  return navigator.serviceWorker.register('/push-sw.js', { scope: '/' })
}

function toSubscriptionInput(payload: PushSubscriptionJSON): ViewerPushSubscriptionInput | null {
  const endpoint = payload.endpoint?.trim()
  const p256dh = payload.keys?.p256dh?.trim()
  const auth = payload.keys?.auth?.trim()
  if (!endpoint || !p256dh || !auth) return null
  return {
    endpoint,
    expirationTime: payload.expirationTime ?? null,
    keys: { p256dh, auth },
  }
}

function urlBase64ToUint8Array(value: string) {
  const padding = '='.repeat((4 - (value.length % 4)) % 4)
  const base64 = (value + padding).replace(/-/g, '+').replace(/_/g, '/')
  const raw = window.atob(base64)
  return Uint8Array.from(raw, (character) => character.charCodeAt(0))
}
