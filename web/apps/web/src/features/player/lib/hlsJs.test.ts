import { shouldRecreateSession } from "@mivo/player-core";
import Hls, { ErrorDetails, ErrorTypes, Events, type ErrorData } from "hls.js";
import { describe, expect, it, vi } from "vitest";

import {
  createHlsJsPlayer,
  hlsConfig,
  maxBufferSeconds,
  maxMaxBufferSeconds,
  toHlsStreamError,
} from "./hlsJs";
import {
  isMediaError,
  isStreamNotReady,
  manifestLoadErrorDetails,
  mediaErrorType,
} from "./hlsPlayer";

function errorData(fields: Partial<ErrorData> = {}): ErrorData {
  return {
    type: ErrorTypes.NETWORK_ERROR,
    details: ErrorDetails.MANIFEST_LOAD_ERROR,
    fatal: true,
    error: new Error("load failed"),
    ...fields,
  };
}

// player-core and the engine spell hls.js codes as literals, because neither may import hls.js
// statically. These tests fail when an hls.js update renames one of them.
describe("hls.js codes the player relies on", () => {
  it("match the literals of player-core and the engine", () => {
    expect(ErrorTypes.NETWORK_ERROR).toBe("networkError");
    expect(ErrorDetails.FRAG_LOAD_ERROR).toBe("fragLoadError");
    expect(ErrorDetails.FRAG_LOAD_TIMEOUT).toBe("fragLoadTimeOut");
    expect(ErrorDetails.MANIFEST_LOAD_ERROR).toBe(manifestLoadErrorDetails);
    expect(ErrorTypes.MEDIA_ERROR).toBe(mediaErrorType);
  });

  it("recognises a fatal media error", () => {
    const { error } = toHlsStreamError(
      errorData({ type: ErrorTypes.MEDIA_ERROR, details: ErrorDetails.BUFFER_APPEND_ERROR }),
    );

    expect(isMediaError(error)).toBe(true);
    expect(isMediaError({ ...error, fatal: false })).toBe(false);
    expect(isMediaError({ ...error, type: ErrorTypes.NETWORK_ERROR })).toBe(false);
  });

  it.each([ErrorDetails.FRAG_LOAD_ERROR, ErrorDetails.FRAG_LOAD_TIMEOUT])(
    "lets a fatal %s recreate the session",
    (details) => {
      const { error } = toHlsStreamError(errorData({ details }));

      expect(shouldRecreateSession({ source: "hls", error })).toBe(true);
    },
  );

  it("recognises a playlist that is not ready yet", () => {
    const { error } = toHlsStreamError(errorData({ response: { url: "index.m3u8", code: 503 } }));

    expect(isStreamNotReady(error)).toBe(true);
    expect(isStreamNotReady({ ...error, fatal: false })).toBe(false);
    expect(isStreamNotReady({ ...error, response: { code: 500 } })).toBe(false);
    expect(isStreamNotReady({ ...error, details: ErrorDetails.LEVEL_LOAD_ERROR })).toBe(false);
  });
});

describe("toHlsStreamError", () => {
  it("keeps the fields recovery needs and reads Retry-After from the request", () => {
    const networkDetails = new Response(null, { status: 503, headers: { "Retry-After": "3" } });
    const response = { url: "index.m3u8", code: 503 };

    expect(toHlsStreamError(errorData({ response, networkDetails }))).toEqual({
      error: { type: "networkError", details: "manifestLoadError", fatal: true, response },
      retryAfterSeconds: 3,
    });
  });
});

describe("createHlsJsPlayer", () => {
  it("bounds the buffer and keeps the worker", () => {
    expect(hlsConfig).toMatchObject({
      enableWorker: true,
      maxBufferLength: maxBufferSeconds,
      maxMaxBufferLength: maxMaxBufferSeconds,
    });
    // Longer than the server's 20 s wait, so its stream_not_ready answer arrives.
    expect(hlsConfig.manifestLoadPolicy?.default.maxLoadTimeMs).toBeGreaterThan(20_000);
    // The engine paces playlist retries by Retry-After; hls.js must not retry on its own.
    expect(hlsConfig.manifestLoadPolicy?.default.errorRetry?.maxNumRetry).toBe(0);
  });

  it("starts an unfinished EVENT playlist at its beginning, not at the live edge", () => {
    expect(hlsConfig.startPosition).toBe(0);
  });

  it("loads hls.js, applies the config and reports its errors", async () => {
    const onError = vi.fn();

    const player = await createHlsJsPlayer(onError);

    expect(player).toBeInstanceOf(Hls);
    const hls = player as Hls;
    expect(hls.config.maxMaxBufferLength).toBe(maxMaxBufferSeconds);
    expect(hls.config.startPosition).toBe(0);
    hls.trigger(Events.ERROR, errorData({ response: { url: "index.m3u8", code: 503 } }));
    expect(onError).toHaveBeenCalledWith(
      {
        error: expect.objectContaining({ details: "manifestLoadError", fatal: true }) as unknown,
        retryAfterSeconds: undefined,
      },
      hls,
    );
    player.destroy();
  });
});
