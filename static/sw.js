/**
 * 서비스 워커 - PWA 지원
 */

const CACHE_NAME = 'drive-duplicates-v1';
const STATIC_CACHE = 'static-v1';
const DYNAMIC_CACHE = 'dynamic-v1';

// 캐시할 정적 파일들
const STATIC_FILES = [
  './',
  './index.html',
  './css/app.css',
  './js/app.js',
  './js/api.js',
  './js/modules/App.js',
  './js/modules/core/EventBus.js',
  './js/modules/core/StateManager.js',
  './js/modules/core/Router.js',
  './manifest.json'
];

// 설치 이벤트
self.addEventListener('install', (event) => {
  console.log('[SW] Installing...');
  
  event.waitUntil(
    caches.open(STATIC_CACHE)
      .then((cache) => {
        console.log('[SW] Caching static files');
        return cache.addAll(STATIC_FILES);
      })
      .catch((error) => {
        console.error('[SW] Cache installation failed:', error);
      })
  );
  
  // 즉시 활성화
  self.skipWaiting();
});

// 활성화 이벤트
self.addEventListener('activate', (event) => {
  console.log('[SW] Activating...');
  
  event.waitUntil(
    caches.keys().then((cacheNames) => {
      return Promise.all(
        cacheNames.map((cacheName) => {
          if (cacheName !== STATIC_CACHE && cacheName !== DYNAMIC_CACHE) {
            console.log('[SW] Deleting old cache:', cacheName);
            return caches.delete(cacheName);
          }
        })
      );
    })
  );
  
  // 모든 클라이언트 제어
  return self.clients.claim();
});

// 페치 이벤트 - 네트워크 우선 전략
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);
  
  // chrome-extension, moz-extension 등 브라우저 확장 프로그램 요청은 무시
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    return;
  }
  
  // API 요청은 캐시하지 않음
  if (url.pathname.startsWith('/api/')) {
    return;
  }
  
  // 정적 파일 캐시 전략
  if (STATIC_FILES.includes(url.pathname)) {
    event.respondWith(
      caches.match(request)
        .then((cachedResponse) => {
          if (cachedResponse) {
            return cachedResponse;
          }
          
          return fetch(request)
            .then((response) => {
              if (response.ok) {
                const responseClone = response.clone();
                const requestUrl = new URL(request.url);
                // http/https 요청만 캐시 저장
                if (requestUrl.protocol === 'http:' || requestUrl.protocol === 'https:') {
                  caches.open(STATIC_CACHE)
                    .then((cache) => cache.put(request, responseClone));
                }
              }
              return response;
            });
        })
    );
  } else {
    // 동적 콘텐츠는 네트워크 우선
    event.respondWith(
      fetch(request)
        .then((response) => {
          if (response.ok) {
            const responseClone = response.clone();
            const requestUrl = new URL(request.url);
            // http/https 요청만 캐시 저장
            if (requestUrl.protocol === 'http:' || requestUrl.protocol === 'https:') {
              caches.open(DYNAMIC_CACHE)
                .then((cache) => cache.put(request, responseClone));
            }
          }
          return response;
        })
        .catch(() => {
          // 네트워크 실패 시 캐시에서 반환
          return caches.match(request).then(cachedResponse => {
            if (cachedResponse) {
              return cachedResponse;
            }
            // 캐시에도 없으면 네트워크 오류 응답 반환
            return new Response('Network error', {
              status: 408,
              statusText: 'Request timeout'
            });
          });
        })
    );
  }
});

// 백그라운드 동기화 (미래 확장용)
self.addEventListener('sync', (event) => {
  if (event.tag === 'background-sync') {
    console.log('[SW] Background sync triggered');
    // 필요시 백그라운드 작업 수행
  }
});

// 푸시 알림 (미래 확장용)
self.addEventListener('push', (event) => {
  if (event.data) {
    const data = event.data.json();
    console.log('[SW] Push notification received:', data);
    
    event.waitUntil(
      self.registration.showNotification(data.title, {
        body: data.body,
        icon: './images/icon-192.png',
        badge: './images/icon-192.png',
        tag: 'drive-duplicates'
      })
    );
  }
});