import { describe, expect, it } from "vitest";

import {
  applyVolumePreference,
  defaultVolume,
  preferenceToSave,
  readVolumePreference,
  volumeStorageKey,
  writeVolumePreference,
} from "./volumePreference";

function storageWith(value: string | null): Storage {
  const storage = window.localStorage;
  storage.clear();
  if (value !== null) {
    storage.setItem(volumeStorageKey, value);
  }
  return storage;
}

describe("readVolumePreference", () => {
  it.each([
    {
      name: "a saved value",
      stored: '{"volume":0.5,"muted":true}',
      expected: { volume: 0.5, muted: true },
    },
    { name: "nothing saved", stored: null, expected: undefined },
    { name: "a volume above 1", stored: '{"volume":2,"muted":false}', expected: undefined },
    { name: "a missing mute state", stored: '{"volume":0.5}', expected: undefined },
    { name: "a number instead of an object", stored: "0.5", expected: undefined },
    { name: "broken JSON", stored: "{", expected: undefined },
  ])("reads $name", ({ stored, expected }) => {
    expect(readVolumePreference(storageWith(stored))).toEqual(expected);
  });

  it("reads nothing without storage", () => {
    expect(readVolumePreference(undefined)).toBeUndefined();
  });
});

describe("writeVolumePreference", () => {
  it("saves a value readVolumePreference reads back", () => {
    const storage = storageWith(null);

    writeVolumePreference(storage, { volume: 0.3, muted: false });

    expect(readVolumePreference(storage)).toEqual({ volume: 0.3, muted: false });
  });

  it("ignores a failing storage", () => {
    const failing = {
      setItem: () => {
        throw new Error("quota");
      },
    } as unknown as Storage;

    expect(() => {
      writeVolumePreference(failing, { volume: 1, muted: false });
    }).not.toThrow();
  });
});

describe("preferenceToSave", () => {
  it.each([
    {
      name: "an audible volume as it is",
      sound: { volume: 0.6, muted: true },
      previous: undefined,
      expected: { volume: 0.6, muted: true },
    },
    {
      name: "volume 0 as muted at the last audible volume",
      sound: { volume: 0, muted: true },
      previous: { volume: 0.7, muted: false },
      expected: { volume: 0.7, muted: true },
    },
    {
      name: "volume 0 without a saved value as muted at the default",
      sound: { volume: 0, muted: true },
      previous: undefined,
      expected: { volume: defaultVolume, muted: true },
    },
  ])("saves $name", ({ sound, previous, expected }) => {
    expect(preferenceToSave(sound, previous)).toEqual(expected);
  });
});

describe("applyVolumePreference", () => {
  it("sets volume and mute state", () => {
    const media = { volume: 1, muted: false };

    applyVolumePreference(media, { volume: 0.2, muted: true });

    expect(media).toEqual({ volume: 0.2, muted: true });
  });
});
