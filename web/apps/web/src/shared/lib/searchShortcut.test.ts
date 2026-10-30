import { describe, expect, it } from "vitest";

import { isSearchShortcut } from "./searchShortcut";

describe("isSearchShortcut", () => {
  it.each([
    { name: "Ctrl+K", init: { key: "k", code: "KeyK", ctrlKey: true }, expected: true },
    { name: "Cmd+K", init: { key: "k", code: "KeyK", metaKey: true }, expected: true },
    { name: "Ctrl+K with Caps Lock", init: { key: "K", ctrlKey: true }, expected: true },
    { name: "Ctrl+K without a key code", init: { key: "k", ctrlKey: true }, expected: true },
    {
      name: "Ctrl+K on a Cyrillic layout",
      init: { key: "л", code: "KeyK", ctrlKey: true },
      expected: true,
    },
    {
      name: "Ctrl+K on Dvorak (physical V)",
      init: { key: "k", code: "KeyV", ctrlKey: true },
      expected: true,
    },
    {
      name: "Ctrl+T on Dvorak (physical K)",
      init: { key: "t", code: "KeyK", ctrlKey: true },
      expected: false,
    },
    {
      name: "Ctrl+K while composing",
      init: { key: "k", code: "KeyK", ctrlKey: true, isComposing: true },
      expected: false,
    },
    { name: "K alone", init: { key: "k", code: "KeyK" }, expected: false },
    { name: "Ctrl+Shift+K", init: { key: "K", ctrlKey: true, shiftKey: true }, expected: false },
    { name: "Ctrl+Alt+K", init: { key: "k", ctrlKey: true, altKey: true }, expected: false },
    { name: "Ctrl+J", init: { key: "j", code: "KeyJ", ctrlKey: true }, expected: false },
    { name: "Ctrl+Enter", init: { key: "Enter", code: "Enter", ctrlKey: true }, expected: false },
  ])("returns $expected for $name", ({ init, expected }) => {
    expect(isSearchShortcut(new KeyboardEvent("keydown", init))).toBe(expected);
  });
});
