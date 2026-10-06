import { useSyncExternalStore } from "react";

export const PUBLIC_THEME_KEY = "public-theme";
export type PublicTheme = "light" | "dark";

const THEME_EVENT = "public-theme-change";

export function getStoredPublicTheme(): PublicTheme {
  if (typeof window === "undefined") return "light";
  const stored = localStorage.getItem(PUBLIC_THEME_KEY);
  return stored === "dark" ? "dark" : "light";
}

export function setStoredPublicTheme(theme: PublicTheme): void {
  localStorage.setItem(PUBLIC_THEME_KEY, theme);
  applyPublicThemeToDocument(theme);
  window.dispatchEvent(new Event(THEME_EVENT));
}

function subscribe(onChange: () => void) {
  window.addEventListener(THEME_EVENT, onChange);
  window.addEventListener("storage", onChange);
  return () => {
    window.removeEventListener(THEME_EVENT, onChange);
    window.removeEventListener("storage", onChange);
  };
}

/** The stored public theme, kept in sync across components and tabs. */
export function usePublicTheme(): PublicTheme {
  return useSyncExternalStore(subscribe, getStoredPublicTheme, () => "light");
}

export function applyPublicThemeToDocument(theme: PublicTheme): void {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

export function clearPublicThemeFromDocument(): void {
  document.documentElement.classList.remove("dark");
}
