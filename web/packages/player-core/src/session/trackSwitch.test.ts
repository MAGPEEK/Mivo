import { describe, expect, it } from "vitest";

import { audio, imageSubtitle, subtitle } from "../tracks/test/tracks";
import type { TrackSession } from "../tracks/trackModel";
import type { TrackSelection } from "./sessionRequest";
import {
  planAudioSwitch,
  planQualitySwitch,
  planSubtitleSwitch,
  reconcileSelection,
  shownSubtitleTrackId,
  type SwitchPlan,
  type SwitchState,
} from "./trackSwitch";

const german = subtitle("s:3");
const english = subtitle("s:4");
const image = imageSubtitle("s:5");
const tracks = [german, english, image];
const audioTracks = [audio(1), audio(2)];

function state(
  selection: TrackSelection = {},
  subtitleDecision?: TrackSession["subtitle"],
): SwitchState {
  return {
    selection,
    session: {
      streams: [
        { kind: "video", sourceIndex: 0, action: "copy", sourceCodec: "h264", targetCodec: "h264" },
        { kind: "audio", sourceIndex: 1, action: "copy", sourceCodec: "aac", targetCodec: "aac" },
      ],
      ...(subtitleDecision === undefined ? {} : { subtitle: subtitleDecision }),
    },
  };
}

const none: SwitchPlan = { kind: "none" };

describe("planAudioSwitch", () => {
  it.each<{ name: string; state: SwitchState; index: number; expected: SwitchPlan }>([
    { name: "the track the session's policy picked", state: state(), index: 1, expected: none },
    {
      name: "the track already chosen",
      state: state({ audioStreamIndex: 2 }),
      index: 2,
      expected: none,
    },
    {
      name: "another track restarts",
      state: state(),
      index: 2,
      expected: { kind: "restart", selection: { audioStreamIndex: 2 } },
    },
    {
      name: "the rest of the selection is kept",
      state: state({ audioStreamIndex: 2, subtitleTrackId: "s:3", maxBitRate: 4_000_000 }),
      index: 1,
      expected: {
        kind: "restart",
        selection: { audioStreamIndex: 1, subtitleTrackId: "s:3", maxBitRate: 4_000_000 },
      },
    },
    { name: "the video stream is no audio track", state: state(), index: 0, expected: none },
    { name: "an index the file does not have", state: state(), index: 7, expected: none },
  ])("$name", ({ state: current, index, expected }) => {
    expect(planAudioSwitch(current, audioTracks, index)).toEqual(expected);
  });
});

describe("planQualitySwitch", () => {
  it.each<{ name: string; state: SwitchState; maxBitRate: number; expected: SwitchPlan }>([
    { name: "original while playing the original", state: state(), maxBitRate: 0, expected: none },
    {
      name: "the same limit",
      state: state({ maxBitRate: 4_000_000 }),
      maxBitRate: 4_000_000,
      expected: none,
    },
    {
      name: "a limit restarts",
      state: state({ audioStreamIndex: 2 }),
      maxBitRate: 8_000_000,
      expected: { kind: "restart", selection: { audioStreamIndex: 2, maxBitRate: 8_000_000 } },
    },
    {
      name: "back to the original restarts",
      state: state({ maxBitRate: 4_000_000 }),
      maxBitRate: 0,
      expected: { kind: "restart", selection: { maxBitRate: 0 } },
    },
  ])("$name", ({ state: current, maxBitRate, expected }) => {
    expect(planQualitySwitch(current, maxBitRate)).toEqual(expected);
  });
});

describe("reconcileSelection", () => {
  it.each<{ name: string; state: SwitchState; expected: TrackSelection }>([
    { name: "nothing chosen", state: state(), expected: {} },
    {
      name: "a text track stays chosen whatever the session says",
      state: state({ subtitleTrackId: "s:3" }, { mode: "off" }),
      expected: { subtitleTrackId: "s:3" },
    },
    {
      name: "a burned-in track the session shows stays chosen",
      state: state({ subtitleTrackId: "s:5" }, { mode: "burn_in", trackId: "s:5" }),
      expected: { subtitleTrackId: "s:5" },
    },
    {
      name: "a burn-in the server could not do becomes off",
      state: state({ audioStreamIndex: 2, subtitleTrackId: "s:5" }, { mode: "off" }),
      expected: { audioStreamIndex: 2, subtitleTrackId: "" },
    },
    {
      name: "a burn-in track without a subtitle decision becomes off",
      state: state({ subtitleTrackId: "s:5" }),
      expected: { subtitleTrackId: "" },
    },
    {
      name: "a burn-in track while another one is burned in becomes off",
      state: state({ subtitleTrackId: "s:5" }, { mode: "burn_in", trackId: "s:6" }),
      expected: { subtitleTrackId: "" },
    },
    {
      name: "off stays off",
      state: state({ subtitleTrackId: "" }),
      expected: { subtitleTrackId: "" },
    },
    {
      name: "a track the file does not list is left alone",
      state: state({ subtitleTrackId: "s:9" }),
      expected: { subtitleTrackId: "s:9" },
    },
  ])("$name", ({ state: current, expected }) => {
    expect(reconcileSelection(current.selection, current.session, tracks)).toEqual(expected);
  });

  it("returns the same selection when nothing changes", () => {
    const current = state({ subtitleTrackId: "s:3" });
    expect(reconcileSelection(current.selection, current.session, tracks)).toBe(current.selection);
  });
});

describe("shownSubtitleTrackId", () => {
  it.each<{ name: string; state: SwitchState; expected: string }>([
    { name: "nothing chosen, no decision", state: state(), expected: "" },
    {
      name: "the policy's pick",
      state: state({}, { mode: "sidecar", trackId: "s:3" }),
      expected: "s:3",
    },
    { name: "the policy's off", state: state({}, { mode: "off" }), expected: "" },
    { name: "a decision without id", state: state({}, { mode: "sidecar" }), expected: "" },
    {
      name: "the viewer's choice wins",
      state: state({ subtitleTrackId: "s:4" }, { mode: "sidecar", trackId: "s:3" }),
      expected: "s:4",
    },
    {
      name: "the viewer's off wins",
      state: state({ subtitleTrackId: "" }, { mode: "sidecar", trackId: "s:3" }),
      expected: "",
    },
    {
      name: "a degraded burn-in shows as off in the menu",
      state: state({ subtitleTrackId: "s:5" }, { mode: "off" }),
      expected: "",
    },
  ])("$name", ({ state: current, expected }) => {
    expect(shownSubtitleTrackId(current, tracks)).toBe(expected);
  });
});

describe("planSubtitleSwitch", () => {
  it.each<{ name: string; state: SwitchState; trackId: string; expected: SwitchPlan }>([
    {
      name: "a text track is swapped without a restart",
      state: state({ audioStreamIndex: 2 }),
      trackId: "s:3",
      expected: {
        kind: "sidecar",
        track: german,
        selection: { audioStreamIndex: 2, subtitleTrackId: "s:3" },
      },
    },
    {
      name: "between text tracks",
      state: state({ subtitleTrackId: "s:3" }, { mode: "sidecar", trackId: "s:3" }),
      trackId: "s:4",
      expected: { kind: "sidecar", track: english, selection: { subtitleTrackId: "s:4" } },
    },
    {
      name: "a text track off",
      state: state({}, { mode: "sidecar", trackId: "s:3" }),
      trackId: "",
      expected: { kind: "sidecar", track: undefined, selection: { subtitleTrackId: "" } },
    },
    {
      name: "the track already shown",
      state: state({ subtitleTrackId: "s:3" }),
      trackId: "s:3",
      expected: none,
    },
    { name: "off while off", state: state(), trackId: "", expected: none },
    { name: "an unknown track", state: state(), trackId: "s:9", expected: none },
    {
      name: "to a burned-in track restarts",
      state: state({ subtitleTrackId: "s:3" }, { mode: "sidecar", trackId: "s:3" }),
      trackId: "s:5",
      expected: { kind: "restart", selection: { subtitleTrackId: "s:5" } },
    },
    {
      name: "from a burned-in track to a text track restarts",
      state: state({ subtitleTrackId: "s:5" }, { mode: "burn_in", trackId: "s:5" }),
      trackId: "s:3",
      expected: { kind: "restart", selection: { subtitleTrackId: "s:3" } },
    },
    {
      name: "a burned-in track off restarts",
      state: state({ subtitleTrackId: "s:5" }, { mode: "burn_in", trackId: "s:5" }),
      trackId: "",
      expected: { kind: "restart", selection: { subtitleTrackId: "" } },
    },
    {
      name: "the burned-in track already shown",
      state: state({}, { mode: "burn_in", trackId: "s:5" }),
      trackId: "s:5",
      expected: none,
    },
    {
      name: "re-selecting a degraded burn-in asks the server again",
      state: state({ subtitleTrackId: "s:5" }, { mode: "off" }),
      trackId: "s:5",
      expected: { kind: "restart", selection: { subtitleTrackId: "s:5" } },
    },
    {
      name: "off after a degraded burn-in changes nothing",
      state: state({ subtitleTrackId: "s:5" }, { mode: "off" }),
      trackId: "",
      expected: none,
    },
    {
      name: "a text track after a degraded burn-in is swapped without a restart",
      state: state({ subtitleTrackId: "s:5" }, { mode: "off" }),
      trackId: "s:3",
      expected: { kind: "sidecar", track: german, selection: { subtitleTrackId: "s:3" } },
    },
  ])("$name", ({ state: current, trackId, expected }) => {
    expect(planSubtitleSwitch(current, tracks, trackId)).toEqual(expected);
  });
});
