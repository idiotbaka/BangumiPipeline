self.addEventListener('push', (event) => {
  let payload = {}
  try {
    payload = event.data ? event.data.json() : {}
  } catch {
    payload = {}
  }
  const title = typeof payload.title === 'string' && payload.title ? payload.title : '番剧更新提醒'
  const body = typeof payload.body === 'string' ? payload.body : '你追的番剧有新集可以观看了。'
  const url = typeof payload.url === 'string' && payload.url.startsWith('/') ? payload.url : '/'
  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      icon: '/favicon.png',
      badge: '/favicon.png',
      tag: typeof payload.tag === 'string' ? payload.tag : 'bangumi-update',
      renotify: false,
      data: { url },
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const targetURL = new URL(event.notification.data?.url || '/', self.location.origin).href
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if (client.url.startsWith(self.location.origin)) {
          return client.navigate(targetURL).then(() => client.focus())
        }
      }
      return self.clients.openWindow(targetURL)
    }),
  )
})
