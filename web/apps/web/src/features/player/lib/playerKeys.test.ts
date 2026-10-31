import { describe, expect, it } from "vitest";

import { playerKeyCommand, scrubberStepSeconds, type PlayerKeyEvent } from "./playerKeys";

function keyEvent(key: string, overrides: Partial<PlayerKeyEvent> = {}): PlayerKeyEvent {
  return {
    key,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    repeat: false,
    target: null,
    ...overrides,
  };
}

function element(html: string): Element {
  const container = document.createElement("div");
  container.innerHTML = html;
  const found = container.querySelector("[data-target]");
  if (found === null) {
    throw new Error("the markup has no [data-target] element");
  }
  return found;
}

describe("playerKeyCommand", () => {
  it.each([
    { key: " ", command: "togglePlay" },
    { key: "k", command: "togglePlay" },
    { key: "K", command: "togglePlay" },
    { key: "ArrowLeft", command: "seekBackward" },
    { key: "ArrowRight", command: "seekForward" },
    { key: "f", command: "toggleFullscreen" },
    { key: "F", command: "toggleFullscreen" },
    { key: "i", command: "toggleStreamInfo" },
    { key: "m", command: "toggleMute" },
    { key: "Escape", command: "escape" },
  ])("maps $key to $command", ({ key, command }) => {
    expect(playerKeyCommand(keyEvent(key))).toBe(command);
  });

  it.each(["a", "Enter", "ArrowUp", "Tab", "F11"])("ignores %s", (key) => {
    expect(playerKeyCommand(keyEvent(key))).toBeUndefined();
  });

  it.each([{ altKey: true }, { ctrlKey: true }, { metaKey: true }])(
    "leaves shortcuts with modifiers to the browser (%o)",
    (modifier) => {
      expect(playerKeyCommand(keyEvent("f", modifier))).toBeUndefined();
    },
  );

  it("leaves Space on a focused button to the button, but never a letter", () => {
    const button = element('<button data-target type="button">Mute</button>');

    expect(playerKeyCommand(keyEvent(" ", { target: button }))).toBeUndefined();
    expect(playerKeyCommand(keyEvent("k", { target: button }))).toBe("togglePlay");
    expect(playerKeyCommand(keyEvent("m", { target: button }))).toBe("toggleMute");
  });

  it.each(["f", "i", "m", "k", " ", "Escape"])("ignores a held %s", (key) => {
    expect(playerKeyCommand(keyEvent(key, { repeat: true }))).toBeUndefined();
  });

  it.each([
    { key: "ArrowLeft", command: "seekBackward" },
    { key: "ArrowRight", command: "seekForward" },
  ])("repeats $key while held", ({ key, command }) => {
    expect(playerKeyCommand(keyEvent(key, { repeat: true }))).toBe(command);
  });

  it("leaves the arrow keys on a focused slider to the slider but plays on Space", () => {
    const slider = element('<input data-target type="range" aria-label="Volume" />');

    expect(playerKeyCommand(keyEvent("ArrowLeft", { target: slider }))).toBeUndefined();
    expect(playerKeyCommand(keyEvent("ArrowRight", { target: slider }))).toBeUndefined();
    expect(playerKeyCommand(keyEvent(" ", { target: slider }))).toBe("togglePlay");
  });

  it("handles every key on elements without a key action of their own", () => {
    const heading = element('<h2 data-target tabindex="-1">Stream info</h2>');

    expect(playerKeyCommand(keyEvent(" ", { target: heading }))).toBe("togglePlay");
    expect(playerKeyCommand(keyEvent("ArrowRight", { target: heading }))).toBe("seekForward");
    expect(playerKeyCommand(keyEvent("Escape", { target: window }))).toBe("escape");
  });
});

describe("scrubberStepSeconds", () => {
  it.each([
    { key: "ArrowLeft", step: -10 },
    { key: "ArrowDown", step: -10 },
    { key: "ArrowRight", step: 10 },
    { key: "ArrowUp", step: 10 },
    { key: "Home", step: undefined },
    { key: "PageUp", step: undefined },
  ])("moves $key by $step s", ({ key, step }) => {
    expect(scrubberStepSeconds(key)).toBe(step);
  });
});
