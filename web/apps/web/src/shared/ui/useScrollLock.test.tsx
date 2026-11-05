import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useScrollLock } from "./useScrollLock";

afterEach(() => {
  document.body.style.overflow = "";
});

describe("useScrollLock", () => {
  it("keeps the page locked until the last of nested locks is released", () => {
    const outer = renderHook(() => {
      useScrollLock();
    });
    const inner = renderHook(() => {
      useScrollLock();
    });
    inner.unmount();
    expect(document.body.style.overflow).toBe("hidden");
    outer.unmount();

    expect(document.body.style.overflow).toBe("");
  });

  it("restores the page when locks are released out of order", () => {
    const first = renderHook(() => {
      useScrollLock();
    });
    const second = renderHook(() => {
      useScrollLock();
    });
    first.unmount();
    expect(document.body.style.overflow).toBe("hidden");
    second.unmount();

    expect(document.body.style.overflow).toBe("");
  });

  it("gives back a non-empty original overflow value", () => {
    document.body.style.overflow = "scroll";
    const first = renderHook(() => {
      useScrollLock();
    });
    const second = renderHook(() => {
      useScrollLock();
    });
    first.unmount();
    second.unmount();

    expect(document.body.style.overflow).toBe("scroll");
  });
});
