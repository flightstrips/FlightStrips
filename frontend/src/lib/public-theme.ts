export const PUBLIC_THEME_KEY = "public-theme";
export type PublicTheme = "light" | "dark";

export function getStoredPublicTheme(): PublicTheme {
  if (typeof window === "undefined") return "light";
  const stored = localStorage.getItem(PUBLIC_THEME_KEY);
  return stored === "dark" ? "dark" : "light";
}

export function setStoredPublicTheme(theme: PublicTheme): void {
  localStorage.setItem(PUBLIC_THEME_KEY, theme);
  applyPublicThemeToDocument(theme);
}

export function applyPublicThemeToDocument(theme: PublicTheme): void {
  document.documentElement.classList.toggle("dark", theme === "dark");
}

export function clearPublicThemeFromDocument(): void {
  document.documentElement.classList.remove("dark");
}
