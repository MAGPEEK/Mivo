// ISO 639-2 codes that name no language: "und" (undetermined) would otherwise come back from
// Intl.DisplayNames as "root", the name of CLDR's base locale.
const undeterminedLanguageCodes: ReadonlySet<string> = new Set(["und"]);

// One formatter per UI language: a detail page names every track, and constructing an Intl
// formatter is far more expensive than a lookup. The app has two UI languages, so the cache
// stays tiny.
const displayNamesByLanguage = new Map<string, Intl.DisplayNames>();

function displayNames(uiLanguage: string): Intl.DisplayNames {
  let names = displayNamesByLanguage.get(uiLanguage);
  if (names === undefined) {
    // "none" makes a tag without a name explicit (undefined) instead of relying on Intl
    // echoing it, so languageName alone decides what is shown.
    names = new Intl.DisplayNames([uiLanguage], { type: "language", fallback: "none" });
    displayNamesByLanguage.set(uiLanguage, names);
  }
  return names;
}

/**
 * Name of a stream's language tag in the UI language, e.g. `ger` → `German` for `en` and
 * `Deutsch` for `de`. Tags without a name (private codes such as `qaa`) and strings that are
 * no language tag at all come back as they are. `undefined` when the stream has no language
 * or marks it as undetermined, so the caller can show a translated "unknown language".
 */
export function languageName(code: string | undefined, uiLanguage: string): string | undefined {
  if (code === undefined || code === "" || undeterminedLanguageCodes.has(code)) {
    return undefined;
  }
  try {
    return displayNames(uiLanguage).of(code) ?? code;
  } catch {
    // RangeError for strings that are no BCP 47 language tag, e.g. "x" or "e1".
    return code;
  }
}
