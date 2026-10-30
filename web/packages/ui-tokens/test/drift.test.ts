import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

import {
  colors,
  fontStackMono,
  fontStackSans,
  radii,
  shadows,
  spacingUnit,
  typeScale,
} from "../src/index";

// Read from disk instead of importing: Vitest replaces CSS imports with empty modules, and the
// test must see the exact bytes the package exports as theme.css.
const themeCss = readFileSync(new URL("../src/theme.css", import.meta.url), "utf8");

const pixelsPerRem = 16;
const colorPrefix = "--color-";

// Reads the custom properties of the @theme block. Namespace resets (`--color-*: initial`) are
// skipped because they declare no token.
function parseThemeDeclarations(css: string): ReadonlyMap<string, string> {
  const withoutComments = css.replace(/\/\*[\s\S]*?\*\//g, "");
  const themeBlock = /@theme\s*\{([^}]*)\}/.exec(withoutComments)?.[1];
  if (themeBlock === undefined) {
    throw new Error("theme.css has no @theme block");
  }
  const declarations = new Map<string, string>();
  for (const [, name = "", value = ""] of themeBlock.matchAll(/(--[\w*-]+)\s*:\s*([^;]+);/g)) {
    const normalizedValue = value.replace(/\s+/g, " ").trim();
    if (normalizedValue !== "initial") {
      declarations.set(name, normalizedValue);
    }
  }
  return declarations;
}

function toKebabCase(camelCase: string): string {
  return camelCase.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`);
}

function toRem(pixels: number): string {
  return `${String(pixels / pixelsPerRem)}rem`;
}

// Hex digits are case-insensitive in CSS. Prettier writes them in lowercase, colors.ts keeps the
// uppercase spelling of ADR 0011, so colour values are compared in uppercase.
function normalizeHex(value: string | undefined): string | undefined {
  return value?.toUpperCase();
}

const declarations = parseThemeDeclarations(themeCss);

const themeColors = [...declarations].filter(([name]) => name.startsWith(colorPrefix));
const tokenColorsByProperty = new Map<string, string>(
  Object.entries(colors).map(([name, value]) => [colorPrefix + toKebabCase(name), value]),
);

describe("theme.css colours", () => {
  it.each(Object.entries(colors))("declares %s with the value from colors.ts", (name, value) => {
    expect(normalizeHex(declarations.get(colorPrefix + toKebabCase(name)))).toBe(value);
  });

  it.each(themeColors)("has %s in colors.ts with the same value", (property, value) => {
    expect(tokenColorsByProperty.get(property)).toBe(normalizeHex(value));
  });

  it("declares exactly as many colours as colors.ts", () => {
    expect(themeColors).toHaveLength(tokenColorsByProperty.size);
  });
});

describe("theme.css typography, spacing, radii and shadows", () => {
  it("uses the TypeScript font stacks", () => {
    expect(declarations.get("--font-sans")).toBe(fontStackSans);
    expect(declarations.get("--font-mono")).toBe(fontStackMono);
  });

  it.each(Object.entries(typeScale.web))("declares text-%s from the web scale", (step, style) => {
    expect(declarations.get(`--text-${step}`)).toBe(toRem(style.fontSize));
    expect(declarations.get(`--text-${step}--line-height`)).toBe(toRem(style.lineHeight));
  });

  it("uses the spacing unit as Tailwind's spacing base", () => {
    expect(declarations.get("--spacing")).toBe(toRem(spacingUnit));
  });

  it.each(Object.entries(radii))("declares radius-%s", (name, pixels) => {
    expect(declarations.get(`--radius-${name}`)).toBe(`${String(pixels)}px`);
  });

  it.each(Object.entries(shadows))("declares shadow %s", (name, value) => {
    expect(declarations.get(`--shadow-${toKebabCase(name)}`)).toBe(value);
  });
});

describe("theme.css declarations", () => {
  // Every variable a TypeScript token maps to. The reverse check below fails when theme.css
  // declares a variable no token backs, in any namespace, not only colours.
  const expectedProperties = [
    ...tokenColorsByProperty.keys(),
    "--font-sans",
    "--font-mono",
    ...Object.keys(typeScale.web).flatMap((step) => [
      `--text-${step}`,
      `--text-${step}--line-height`,
    ]),
    "--spacing",
    ...Object.keys(radii).map((name) => `--radius-${name}`),
    ...Object.keys(shadows).map((name) => `--shadow-${toKebabCase(name)}`),
  ];

  it("declares exactly the variables of the TypeScript tokens", () => {
    expect([...declarations.keys()].toSorted()).toEqual(expectedProperties.toSorted());
  });
});
