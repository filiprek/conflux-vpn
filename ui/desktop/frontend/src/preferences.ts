export type Theme = "light" | "dark" | "system";
export type Language = "en" | "pl";
export interface Preferences {
  language: Language;
  theme: Theme;
}
export function loadPreferences(): Preferences {
  try {
    const value: unknown = JSON.parse(
      localStorage.getItem("conflux.preferences") ?? "{}",
    );
    if (typeof value === "object" && value !== null) {
      const v = value as Record<string, unknown>;
      return {
        language: v.language === "pl" ? "pl" : "en",
        theme: v.theme === "light" || v.theme === "dark" ? v.theme : "system",
      };
    }
  } catch {
    /* Damaged local preferences are safe to reset. */
  }
  return { language: "en", theme: "system" };
}
export function resolveTheme(theme: Theme, dark: boolean): "light" | "dark" {
  return theme === "system" ? (dark ? "dark" : "light") : theme;
}
export function savePreferences(p: Preferences): void {
  try {
    localStorage.setItem(
      "conflux.preferences",
      JSON.stringify({ language: p.language, theme: p.theme }),
    );
  } catch {
    /* Preferences remain usable when local storage is restricted. */
  }
}
