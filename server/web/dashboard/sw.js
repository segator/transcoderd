"use strict";
// This template is versioned and populated by the embedded server.
const CACHE_PREFIX = "transcoderd-dashboard-public-";
const CACHE_NAME = CACHE_PREFIX + "__DASHBOARD_CACHE_VERSION__";
const PUBLIC_PATHS = __DASHBOARD_PUBLIC_PATHS__;
const OFFLINE_PATH = "/dashboard/offline.html";

self.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE_NAME);
      await cache.addAll(
        PUBLIC_PATHS.map(
          (path) =>
            new Request(path, {
              credentials: "omit",
              cache: "reload",
            }),
        ),
      );
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(
        keys
          .filter((key) => key.startsWith(CACHE_PREFIX) && key !== CACHE_NAME)
          .map((key) => caches.delete(key)),
      );
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  const url = new URL(request.url);
  if (
    request.method !== "GET" ||
    url.origin !== self.location.origin ||
    !url.pathname.startsWith("/dashboard/") ||
    url.pathname.startsWith("/dashboard/api/") ||
    request.headers.has("Authorization")
  )
    return;

  if (request.mode === "navigate") {
    // Never cache pages, redirects or login responses. Online auth stays authoritative.
    event.respondWith(
      fetch(request).catch(async () => {
        const offline = await caches.match(OFFLINE_PATH, {
          cacheName: CACHE_NAME,
        });
        return (
          offline ||
          new Response(
            "Sin conexión. Vuelve a conectar para abrir TranscoderD.",
            {
              status: 503,
              headers: {
                "Content-Type": "text/plain; charset=utf-8",
                "Cache-Control": "no-store",
              },
            },
          )
        );
      }),
    );
    return;
  }
  if (url.search || !PUBLIC_PATHS.includes(url.pathname)) return;
  // No runtime writes: only the public install allowlist can enter CacheStorage.
  event.respondWith(
    fetch(request).catch(async () => {
      const cached = await caches.match(url.pathname, {
        cacheName: CACHE_NAME,
      });
      if (cached) return cached;
      throw new Error("Public dashboard asset unavailable");
    }),
  );
});
