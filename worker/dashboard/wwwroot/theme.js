(() => {
  const root = document.documentElement;
  const browserColor = document.querySelector('meta[name="theme-color"]');
  const storageKey = "transcoderd.theme";

  function applyTheme(theme) {
    root.dataset.theme = theme;
    browserColor.content = browserColor.dataset[theme];
  }

  try {
    const saved = localStorage.getItem(storageKey);
    if (saved === "light" || saved === "dark") applyTheme(saved);
  } catch {}

  document.addEventListener("DOMContentLoaded", () => {
    const button = document.querySelector("[data-theme-toggle]");
    function updateButton() {
      const dark = root.dataset.theme === "dark";
      button.setAttribute("aria-pressed", String(dark));
      button.title = dark ? "Switch to light theme" : "Switch to dark theme";
      button.firstElementChild.textContent = dark ? "☀" : "☾";
    }

    updateButton();
    button.hidden = false;
    button.addEventListener("click", () => {
      const theme = root.dataset.theme === "dark" ? "light" : "dark";
      applyTheme(theme);
      updateButton();
      try {
        localStorage.setItem(storageKey, theme);
      } catch {}
    });
  });
})();
