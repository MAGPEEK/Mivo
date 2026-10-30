import { act, render } from "@testing-library/react";
import { createInstance } from "i18next";
import { describe, expect, it } from "vitest";

import { createI18n } from "./createI18n";
import { bindDocumentLanguage, useDocumentLanguage } from "./documentLanguage";

describe("bindDocumentLanguage", () => {
  it("sets lang to the active language right away", () => {
    const element = document.createElement("html");

    bindDocumentLanguage(createI18n(["de-AT"]), element);

    expect(element.lang).toBe("de");
  });

  it("follows language changes until it is stopped", async () => {
    const i18n = createI18n(["en"]);
    const element = document.createElement("html");
    const stop = bindDocumentLanguage(i18n, element);

    await i18n.changeLanguage("de");
    expect(element.lang).toBe("de");

    stop();
    await i18n.changeLanguage("en");
    expect(element.lang).toBe("de");
  });

  it("uses the resolved language for an unsupported request", async () => {
    const i18n = createI18n(["de"]);
    const element = document.createElement("html");
    bindDocumentLanguage(i18n, element);

    await i18n.changeLanguage("fr");

    expect(element.lang).toBe("en");
  });

  it("falls back to English while the instance has no language yet", () => {
    const element = document.createElement("html");

    bindDocumentLanguage(createInstance(), element);

    expect(element.lang).toBe("en");
  });
});

function LanguageProbe({ i18n }: { readonly i18n: ReturnType<typeof createI18n> }) {
  useDocumentLanguage(i18n);
  return null;
}

describe("useDocumentLanguage", () => {
  it("binds <html lang> while mounted", async () => {
    const i18n = createI18n(["de"]);
    const { unmount } = render(<LanguageProbe i18n={i18n} />);
    expect(document.documentElement.lang).toBe("de");

    await act(() => i18n.changeLanguage("en"));
    expect(document.documentElement.lang).toBe("en");

    unmount();
    await act(() => i18n.changeLanguage("de"));
    expect(document.documentElement.lang).toBe("en");
  });
});
