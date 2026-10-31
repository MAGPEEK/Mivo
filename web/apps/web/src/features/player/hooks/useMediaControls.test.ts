import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { initialMediaState } from "../lib/mediaState";
import { useMediaControls } from "./useMediaControls";

describe("useMediaControls", () => {
  it("does nothing before the video element exists", () => {
    const { result } = renderHook(() => useMediaControls(null, undefined));

    act(() => {
      result.current.togglePlay();
      result.current.toggleMute();
      result.current.setVolume(0.5);
    });

    expect(result.current).toMatchObject(initialMediaState);
  });
});
