# Design

Use the existing embedded Go HTTP router and assets. Public manifest, worker and icons are reachable before login, contain no runtime information, and retain explicit CSP/no-store headers. Set worker-src and manifest-src to self. The manifest's stable id/start URL/scope are /dashboard/; use standalone display, Spanish metadata and branded regular/maskable PNGs plus Apple icon.

Compute the worker cache identifier from its template and every allowlisted embedded asset. Precache only public assets with omitted credentials; do not store requests at runtime. Navigation is network-only with an offline reconnection fallback on actual network failure. Never intercept APIs, non-GET, authorized requests or paths outside the dashboard; never cache login or authenticated HTML. Public asset fetches use network first and cached fallback. Activation deletes only this dashboard's older caches and claims clients without forcing reload.

Register from login and dashboard on secure contexts, without making login depend on registration. Show installation controls only when beforeinstallprompt is available; hide in standalone mode and after appinstalled. On a subsequent controller change, offer an explicit reload. Document HTTPS/localhost and platform menu-based installation, and the intentionally live-only monitoring/player features.

Keep the local preview separate from production: synthetic fixture data, test clips and independent worker/API token, while the actual embedded dashboard runs on localhost. Leave it running for the user after testing. Update the existing open PR with the PWA changes after HTTP, lifecycle, manifest/installability, offline/logout and responsive browser checks pass.
