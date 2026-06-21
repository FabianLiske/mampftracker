const STATIC_CACHE = 'mampftracker-static-v1'
const STATIC_PATHS = [
  '/app.webmanifest',
  '/icons/icon-192.png',
  '/icons/icon-512.png',
  '/icons/icon-maskable-512.png',
  '/icons/apple-touch-icon.png',
]

self.addEventListener('install', event => {
  event.waitUntil(caches.open(STATIC_CACHE).then(cache => cache.addAll(STATIC_PATHS)))
  self.skipWaiting()
})

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(keys.filter(key => key !== STATIC_CACHE).map(key => caches.delete(key))))
      .then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', event => {
  const url = new URL(event.request.url)
  if (event.request.method !== 'GET' || url.origin !== self.location.origin) return

  // API and HTML always come from the server. This avoids stale personal data
  // and preserves the HTTP Basic Auth challenge.
  if (url.pathname.startsWith('/api/') || event.request.mode === 'navigate') {
    event.respondWith(fetch(event.request))
    return
  }

  event.respondWith(
    caches.match(event.request).then(cached => {
      const fresh = fetch(event.request).then(response => {
        if (response.ok) {
          const copy = response.clone()
          void caches.open(STATIC_CACHE).then(cache => cache.put(event.request, copy))
        }
        return response
      })
      return cached || fresh
    }),
  )
})
