import { describe, expect, it, vi } from "vitest";

import { createLocalLayoutStore, libraryLayoutStorageKey } from "./layoutStore";

const layout = { order: ["b", "a"], unpinned: ["a"] };

function failingStorage(): Storage {
  const fail = () => {
    throw new Error("blocked");
  };
  return {
    length: 0,
    clear: fail,
    getItem: fail,
    key: fail,
    removeItem: fail,
    setItem: fail,
  };
}

describe("createLocalLayoutStore", () => {
  it("returns the default layout when nothing is saved", () => {
    expect(createLocalLayoutStore(window.localStorage).read()).toBeUndefined();
  });

  it("falls back to the default layout for a corrupt value", () => {
    window.localStorage.setItem(libraryLayoutStorageKey, "{not json");

    expect(createLocalLayoutStore(window.localStorage).read()).toBeUndefined();
  });

  it("saves a layout under a versioned value another store instance reads", () => {
    createLocalLayoutStore(window.localStorage).write(layout);

    expect(window.localStorage.getItem(libraryLayoutStorageKey)).toContain('"version":1');
    expect(createLocalLayoutStore(window.localStorage).read()).toEqual(layout);
  });

  it("returns the same object while the saved value is unchanged", () => {
    const store = createLocalLayoutStore(window.localStorage);
    store.write(layout);

    expect(store.read()).toBe(store.read());
  });

  it("removes the saved value on reset", () => {
    const store = createLocalLayoutStore(window.localStorage);
    store.write(layout);
    store.write(undefined);

    expect(window.localStorage.getItem(libraryLayoutStorageKey)).toBeNull();
    expect(store.read()).toBeUndefined();
  });

  it("notifies the subscribers of every store instance until they unsubscribe", () => {
    const onChange = vi.fn();
    const unsubscribe = createLocalLayoutStore(window.localStorage).subscribe(onChange);

    createLocalLayoutStore(window.localStorage).write(layout);
    unsubscribe();
    createLocalLayoutStore(window.localStorage).write(undefined);

    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("notifies on changes from other tabs to its key only", () => {
    const onChange = vi.fn();
    const unsubscribe = createLocalLayoutStore(window.localStorage).subscribe(onChange);

    window.dispatchEvent(new StorageEvent("storage", { key: "other" }));
    window.dispatchEvent(new StorageEvent("storage", { key: libraryLayoutStorageKey }));
    unsubscribe();

    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it.each([
    { name: "a blocked storage", storage: failingStorage() },
    { name: "no storage", storage: undefined },
  ])("keeps the layout for the session with $name", ({ storage }) => {
    const store = createLocalLayoutStore(storage);

    expect(store.read()).toBeUndefined();
    store.write(layout);

    expect(store.read()).toEqual(layout);
  });

  it("shares an unsaved layout and its reset with subscribed stores of a blocked storage", () => {
    const sidebarStore = createLocalLayoutStore(failingStorage());
    const unsubscribe = sidebarStore.subscribe(() => undefined);
    const settingsStore = createLocalLayoutStore(failingStorage());

    sidebarStore.write(layout);
    settingsStore.write(undefined);
    const afterReset = sidebarStore.read();
    settingsStore.write(layout);
    const afterWrite = sidebarStore.read();
    unsubscribe();

    expect(afterReset).toBeUndefined();
    expect(afterWrite).toEqual(layout);
  });
});
