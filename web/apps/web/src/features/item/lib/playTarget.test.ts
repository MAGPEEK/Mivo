import type { ItemDetail, MediaFile } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import { mediaFile } from "../test/fakeItemServer";
import { playTarget, type PlayTarget } from "./playTarget";

function itemWith(files: MediaFile[]): ItemDetail {
  return { id: "item-1", libraryId: "library-1", title: "Title", files };
}

describe("playTarget", () => {
  it.each<{ name: string; files: MediaFile[]; expected: PlayTarget }>([
    {
      name: "plays the primary file",
      files: [mediaFile({ id: "file-a" }), mediaFile({ id: "file-b" })],
      expected: { kind: "playable", fileId: "file-a" },
    },
    {
      name: "explains an item without files",
      files: [],
      expected: { kind: "unavailable", reason: "noFile" },
    },
    {
      name: "explains a failed probe",
      files: [mediaFile({ available: false, errorCode: "probe_failed" })],
      expected: { kind: "unavailable", reason: "probeFailed" },
    },
    {
      name: "explains a probe timeout",
      files: [mediaFile({ available: false, errorCode: "probe_timed_out" })],
      expected: { kind: "unavailable", reason: "probeTimedOut" },
    },
    {
      name: "falls back for error codes of later server versions",
      files: [mediaFile({ available: false, errorCode: "drm_protected" })],
      expected: { kind: "unavailable", reason: "unknown" },
    },
    {
      name: "falls back when the server sends no error code",
      files: [mediaFile({ available: false })],
      expected: { kind: "unavailable", reason: "unknown" },
    },
  ])("$name", ({ files, expected }) => {
    expect(playTarget(itemWith(files))).toEqual(expected);
  });
});
