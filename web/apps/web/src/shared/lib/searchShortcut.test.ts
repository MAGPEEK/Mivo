import { describe, expect, it } from "vitest";

import { isSearchShortcut } from "./searchShortcut";

describe("isSearchShortcut", () => {
  it.each([
    { name: "Ctrl+K", init: { key: "k", ctrlKey: true }, expected: true },
    { name: "Cmd+K", init: { key: "k", metaKey: true }, expected: true },
    { name: "Ctrl+K with Caps Lock", init: { key: "K", ctrlKey: true }, expected: true },
    { name: "K alone", init: { key: "k" }, expected: false },
    { name: "Ctrl+Shift+K", init: { key: "K", ctrlKey: true, shiftKey: true }, expected: false },
    { name: "Ctrl+Alt+K", init: { key: "k", ctrlKey: true, altKey: true }, expected: false },
    { name: "Ctrl+J", init: { key: "j", ctrlKey: true }, expected: false },
  ])("returns $expected for $name", ({ init, expected }) => {
    expect(isSearchShortcut(new KeyboardEvent("keydown", init))).toBe(expected);
  });
});
