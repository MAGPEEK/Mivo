import { describe, expect, it } from "vitest";

import { formatFileSize } from "./fileSize";

describe("formatFileSize", () => {
  it.each([
    { bytes: 0, uiLanguage: "en", expected: "0 byte" },
    { bytes: 512, uiLanguage: "en", expected: "512 byte" },
    { bytes: 1500, uiLanguage: "en", expected: "1.5 kB" },
    { bytes: 276_134_947, uiLanguage: "en", expected: "276.1 MB" },
    { bytes: 276_134_947, uiLanguage: "de", expected: "276,1 MB" },
    { bytes: 4_700_000_000, uiLanguage: "en", expected: "4.7 GB" },
    // Terabytes are the largest unit; larger sizes keep counting in it.
    { bytes: 2_500_000_000_000_000, uiLanguage: "en", expected: "2,500 TB" },
  ])("formats $bytes bytes in $uiLanguage as $expected", ({ bytes, uiLanguage, expected }) => {
    expect(formatFileSize(bytes, uiLanguage)).toBe(expected);
  });
});
