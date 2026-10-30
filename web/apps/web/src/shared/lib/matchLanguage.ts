/**
 * Picks the first supported language from BCP 47 tags in preference order, such as
 * `navigator.languages`. Tags compare case-insensitively, and a regional tag like `de-AT`
 * matches its base language `de`. Returns `fallback` when no tag matches.
 */
export function matchLanguage<Language extends string>(
  preferred: readonly string[],
  supported: readonly Language[],
  fallback: Language,
): Language {
  const byLowerCase = new Map(supported.map((language) => [language.toLowerCase(), language]));
  for (const tag of preferred) {
    const normalized = tag.trim().toLowerCase();
    const [base = ""] = normalized.split("-");
    const match = byLowerCase.get(normalized) ?? byLowerCase.get(base);
    if (match !== undefined) {
      return match;
    }
  }
  return fallback;
}
