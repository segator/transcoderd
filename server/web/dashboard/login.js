"use strict";
const form = document.getElementById("login-form");
const token = document.getElementById("token");
const error = document.getElementById("login-error");
document.getElementById("toggle-token").addEventListener("click", (event) => {
  const visible = token.type === "password";
  token.type = visible ? "text" : "password";
  event.currentTarget.textContent = visible ? "Hide" : "View";
  event.currentTarget.setAttribute("aria-pressed", String(visible));
  event.currentTarget.setAttribute(
    "aria-label",
    visible ? "Hide token" : "Show token",
  );
});
form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = form.querySelector("[type=submit]");
  button.disabled = true;
  error.textContent = "";
  try {
    const response = await fetch("/dashboard/api/login", {
      method: "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-Dashboard-Request": "1",
      },
      body: JSON.stringify({ token: token.value }),
      signal: AbortSignal.timeout(15000),
    });
    if (!response.ok)
      throw new Error(
        response.status === 401
          ? "Incorrect token. Please try again."
          : "Could not sign in. Please try again.",
      );
    token.value = "";
    window.location.replace("/dashboard/");
  } catch (e) {
    error.textContent =
      e.name === "TypeError" || e.name === "TimeoutError"
        ? "Cannot connect to the server. Please try again."
        : e.message;
    button.disabled = false;
  }
});
