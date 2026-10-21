import type { Category } from "./api";

// A category's name in the language the app is showing: the Arabic name when the UI is
// Arabic and one exists, otherwise the English name. `fallback` is the name the server
// sent with an account/payment, used if the category isn't in the list (yet).
export function localCategoryName(categories: Category[], id: string | undefined | null, fallback: string | undefined | null, lang: string): string {
  const c = id ? categories.find((x) => x.id === id) : undefined;
  if (!c) return fallback ?? "";
  return lang === "ar" && c.name_ar ? c.name_ar : c.name;
}
