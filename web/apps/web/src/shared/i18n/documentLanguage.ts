import type { i18n as I18n } from "i18next";
import { useEffect } from "react";

import { fallbackLanguage } from "./languages";

/**
 * Keeps the `lang` attribute of `element` (normally `<html>`) equal to the active language,
 * so screen readers and hyphenation use the right language. Returns a function that stops
 * following language changes.
 */
export function bindDocumentLanguage(i18n: I18n, element: HTMLElement): () => void {
  const applyLanguage = () => {
    element.lang = i18n.resolvedLanguage ?? fallbackLanguage;
  };
  applyLanguage();
  i18n.on("languageChanged", applyLanguage);
  return () => {
    i18n.off("languageChanged", applyLanguage);
  };
}

/** Binds `<html lang>` to the active language while the calling component is mounted. */
export function useDocumentLanguage(i18n: I18n): void {
  useEffect(() => bindDocumentLanguage(i18n, document.documentElement), [i18n]);
}
