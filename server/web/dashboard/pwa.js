"use strict";
(() => {
  const installButton = document.getElementById("pwa-install");
  const updateButton = document.getElementById("pwa-update");
  const standalone = () =>
    matchMedia("(display-mode: standalone)").matches ||
    navigator.standalone === true;
  let installPrompt = null;
  window.addEventListener("beforeinstallprompt", (event) => {
    if (!installButton || standalone()) return;
    event.preventDefault();
    installPrompt = event;
    installButton.hidden = false;
  });
  window.addEventListener("appinstalled", () => {
    installPrompt = null;
    if (installButton) installButton.hidden = true;
  });
  installButton?.addEventListener("click", async () => {
    if (!installPrompt) return;
    const prompt = installPrompt;
    installPrompt = null;
    installButton.hidden = true;
    try {
      await prompt.prompt();
      await prompt.userChoice;
    } catch {
      /* Installation remains optional; normal dashboard access continues. */
    }
  });
  updateButton?.addEventListener("click", () => location.reload());
  if (!("serviceWorker" in navigator) || !window.isSecureContext) return;
  const initialController = navigator.serviceWorker.controller;
  navigator.serviceWorker.addEventListener("controllerchange", () => {
    if (
      initialController &&
      navigator.serviceWorker.controller !== initialController &&
      updateButton
    )
      updateButton.hidden = false;
  });
  const register = () =>
    navigator.serviceWorker
      .register("/dashboard/sw.js", {
        scope: "/dashboard/",
        updateViaCache: "none",
      })
      .catch(() => {
        /* PWA availability must never block login or live controls. */
      });
  if (document.readyState === "complete") register();
  else window.addEventListener("load", register, { once: true });
})();
