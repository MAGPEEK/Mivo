import type { ItemDetail, MediaFile, SubtitleTrack } from "@mivo/api-client";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../test/axe";
import { readPlayChoice } from "../../player";
import {
  hdrMovie,
  itemServer,
  mediaFile,
  stream,
  type MediaFileFields,
} from "../test/fakeItemServer";
import { renderItemPage } from "../test/renderItemPage";

const textTrack: SubtitleTrack = {
  id: "s:3",
  language: "eng",
  format: "subrip",
  forced: true,
  sdh: false,
  default: false,
  external: false,
  textBased: true,
  url: "/api/v1/subtitles/s3.vtt",
};
const imageTrack: SubtitleTrack = {
  id: "s:4",
  language: "ger",
  format: "pgs",
  forced: false,
  sdh: false,
  default: false,
  external: false,
  textBased: false,
};

function file(id: string, fields: MediaFileFields = {}): MediaFile {
  return mediaFile({
    id,
    container: "mkv",
    width: 1920,
    height: 1080,
    bitRate: 8_200_000,
    streams: [
      stream({ kind: "video", codec: "h264", width: 1920, height: 1080 }),
      stream({ kind: "audio", codec: "ac3", channels: 6, language: "ger", default: true }),
      stream({ kind: "audio", codec: "aac", channels: 2, language: "eng" }),
      stream({ kind: "subtitle", codec: "subrip", language: "eng", forced: true }),
      stream({ kind: "subtitle", codec: "hdmv_pgs_subtitle", language: "ger" }),
    ],
    subtitleTracks: [textTrack, imageTrack],
    ...fields,
  });
}

function movie(files: readonly MediaFile[]): ItemDetail {
  return { ...hdrMovie, files: [...files] };
}

const twoVersions = movie([
  file("file-1", { versionLabel: "1080p cut" }),
  file("file-2", {
    versionLabel: "4K",
    width: 3840,
    height: 2160,
    bitRate: 58_000_000,
    container: "mp4",
    streams: [
      stream({ kind: "video", codec: "hevc", width: 3840, height: 2160 }),
      stream({ kind: "audio", codec: "eac3", channels: 6, language: "eng" }),
    ],
    subtitleTracks: [],
  }),
]);

async function findMain(): Promise<HTMLElement> {
  return await screen.findByRole("main");
}

async function playLink(main: HTMLElement): Promise<HTMLElement> {
  return await within(main).findByRole("link", { name: "Play" });
}

describe("detail selectors", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });
  afterEach(() => {
    cleanup();
  });

  it("shows no selector for one version with one audio track and no subtitles", async () => {
    const plain = movie([
      file("file-1", {
        streams: [
          stream({ kind: "video", codec: "h264" }),
          stream({ kind: "audio", codec: "aac", language: "eng" }),
        ],
        subtitleTracks: [],
      }),
    ]);
    renderItemPage("/items/item-1", { fetch: itemServer([plain]) });
    const main = await findMain();
    await playLink(main);

    expect(within(main).queryByRole("group", { name: "Choose what to play" })).toBeNull();
  });

  it("offers the versions with label, resolution and bit rate", async () => {
    renderItemPage("/items/item-1", { fetch: itemServer([twoVersions]) });
    const main = await findMain();

    const version = await within(main).findByRole<HTMLSelectElement>("combobox", {
      name: "Version",
    });
    expect(Array.from(version.options).map((option) => option.textContent)).toEqual([
      "1080p cut · 1080p · 8.2 Mbit/s",
      "4K · 4K · 58 Mbit/s",
    ]);
    expect(version.value).toBe("file-1");
  });

  it("preselects the first version in the server's order and does not rank them itself", async () => {
    // The server lists the best version first (ADR 0022); here the lower quality comes first.
    const lowerFirst = movie([...twoVersions.files].reverse());
    const higherFirst = movie([...twoVersions.files]);
    const wanted = [
      { item: lowerFirst, first: "file-2" },
      { item: higherFirst, first: "file-1" },
    ];
    for (const { item, first } of wanted) {
      renderItemPage("/items/item-1", { fetch: itemServer([item]) });
      const main = await findMain();
      const version = await within(main).findByRole<HTMLSelectElement>("combobox", {
        name: "Version",
      });

      expect(version.value).toBe(first);
      expect(item.files[0]?.id).toBe(first);
      cleanup();
    }
  });

  it("shows no selectors for an item that cannot be played", async () => {
    // Play follows the primary file; a playable second file with tracks must not bring the
    // selectors back.
    const unavailable = movie([
      file("file-1", { available: false, errorCode: "probe_failed" }),
      file("file-2"),
    ]);
    renderItemPage("/items/item-1", { fetch: itemServer([unavailable]) });
    const main = await findMain();

    expect(await within(main).findByText(/could not read it/)).toBeTruthy();
    expect(within(main).queryByRole("group", { name: "Choose what to play" })).toBeNull();
    expect(within(main).queryByRole("combobox")).toBeNull();
  });

  it("updates chips, tracks and the Play link with the selected version", async () => {
    renderItemPage("/items/item-1", { fetch: itemServer([twoVersions]) });
    const main = await findMain();
    const version = await within(main).findByRole("combobox", { name: "Version" });
    expect(within(main).getByRole("list", { name: "Technical details" }).textContent).toContain(
      "H.264",
    );

    fireEvent.change(version, { target: { value: "file-2" } });

    const chips = within(main).getByRole("list", { name: "Technical details" });
    expect(chips.textContent).toContain("HEVC");
    expect(chips.textContent).not.toContain("H.264");
    expect((await playLink(main)).getAttribute("href")).toBe("/items/item-1/play?fileId=file-2");
    // One audio track and no subtitle track left: those pickers are gone.
    expect(within(main).queryByRole("combobox", { name: "Audio" })).toBeNull();
    expect(within(main).queryByRole("combobox", { name: "Subtitles" })).toBeNull();
  });

  it("preselects the server's default audio and subtitle track", async () => {
    const item = movie([
      file("file-1", { defaultAudioStreamIndex: 2, defaultSubtitleTrackId: "s:3" }),
    ]);
    renderItemPage("/items/item-1", { fetch: itemServer([item]) });
    const main = await findMain();

    const audio = await within(main).findByRole<HTMLSelectElement>("combobox", { name: "Audio" });
    const subtitles = within(main).getByRole<HTMLSelectElement>("combobox", { name: "Subtitles" });
    expect(audio.value).toBe("2");
    expect(subtitles.value).toBe("s:3");
  });

  it("falls back to the player's policy and to Off without a server default", async () => {
    renderItemPage("/items/item-1", { fetch: itemServer([movie([file("file-1")])]) });
    const main = await findMain();

    const audio = await within(main).findByRole<HTMLSelectElement>("combobox", { name: "Audio" });
    const subtitles = within(main).getByRole<HTMLSelectElement>("combobox", { name: "Subtitles" });
    // The German track is flagged default; no forced track in German exists, so no subtitle.
    expect(audio.value).toBe("1");
    expect(subtitles.value).toBe("");
    expect(Array.from(subtitles.options).map((option) => option.textContent)).toEqual([
      "Off",
      "English · SUBRIP · Forced",
      "German · PGS · burned in (transcodes)",
    ]);
  });

  it("reports a null default subtitle as Off", async () => {
    const item = movie([file("file-1", { defaultSubtitleTrackId: null })]);
    renderItemPage("/items/item-1", { fetch: itemServer([item]) });
    const main = await findMain();

    const subtitles = await within(main).findByRole<HTMLSelectElement>("combobox", {
      name: "Subtitles",
    });
    expect(subtitles.value).toBe("");
  });

  it("hands the picked tracks to the player in the navigation state, not the URL", async () => {
    const { router } = renderItemPage("/items/item-1", {
      fetch: itemServer([movie([file("file-1")])]),
    });
    const main = await findMain();
    fireEvent.change(await within(main).findByRole("combobox", { name: "Audio" }), {
      target: { value: "2" },
    });
    fireEvent.change(within(main).getByRole("combobox", { name: "Subtitles" }), {
      target: { value: "s:3" },
    });

    const play = await playLink(main);
    expect(play.getAttribute("href")).toBe("/items/item-1/play?fileId=file-1");
    fireEvent.click(play);

    await screen.findByRole("heading", { level: 1, name: "Player" });
    expect(readPlayChoice(router.state.location.state, "item-1")).toEqual({
      audioStreamIndex: 2,
      subtitleTrackId: "s:3",
    });
  });

  it("moves the subtitle default with the picked audio and hands it over", async () => {
    const { router } = renderItemPage("/items/item-1", {
      fetch: itemServer([movie([file("file-1")])]),
    });
    const main = await findMain();
    const subtitles = await within(main).findByRole<HTMLSelectElement>("combobox", {
      name: "Subtitles",
    });
    expect(subtitles.value).toBe("");

    fireEvent.change(within(main).getByRole("combobox", { name: "Audio" }), {
      target: { value: "2" },
    });

    // The forced English track goes with the English audio.
    expect(subtitles.value).toBe("s:3");
    fireEvent.click(await playLink(main));
    await screen.findByRole("heading", { level: 1, name: "Player" });
    expect(readPlayChoice(router.state.location.state, "item-1")).toEqual({
      audioStreamIndex: 2,
      subtitleTrackId: "s:3",
    });
  });

  it("hands nothing over for tracks the viewer left alone", async () => {
    const { router } = renderItemPage("/items/item-1", {
      fetch: itemServer([movie([file("file-1")])]),
    });
    const main = await findMain();
    fireEvent.click(await playLink(main));

    await screen.findByRole("heading", { level: 1, name: "Player" });
    expect(readPlayChoice(router.state.location.state, "item-1")).toEqual({});
  });

  it("hands Off over as an empty subtitle id", async () => {
    const { router } = renderItemPage("/items/item-1", {
      fetch: itemServer([movie([file("file-1", { defaultSubtitleTrackId: "s:3" })])]),
    });
    const main = await findMain();
    fireEvent.change(await within(main).findByRole("combobox", { name: "Subtitles" }), {
      target: { value: "" },
    });
    fireEvent.click(await playLink(main));

    await screen.findByRole("heading", { level: 1, name: "Player" });
    expect(readPlayChoice(router.state.location.state, "item-1")).toEqual({ subtitleTrackId: "" });
  });

  it("remembers the choice per item on the device", async () => {
    const item = movie([file("file-1")]);
    renderItemPage("/items/item-1", { fetch: itemServer([item]) });
    const first = await findMain();
    fireEvent.change(await within(first).findByRole("combobox", { name: "Audio" }), {
      target: { value: "2" },
    });
    cleanup();

    renderItemPage("/items/item-1", { fetch: itemServer([item]) });
    const again = await findMain();
    const audio = await within(again).findByRole<HTMLSelectElement>("combobox", { name: "Audio" });

    await waitFor(() => {
      expect(audio.value).toBe("2");
    });
  });

  it("ignores a remembered track the file no longer has", async () => {
    window.localStorage.setItem(
      "mivo.playChoices",
      JSON.stringify([
        { itemId: "item-1", choice: { fileId: "file-1", audio: "9", subtitle: "s:99" } },
      ]),
    );
    renderItemPage("/items/item-1", { fetch: itemServer([movie([file("file-1")])]) });
    const main = await findMain();

    const audio = await within(main).findByRole<HTMLSelectElement>("combobox", { name: "Audio" });
    expect(audio.value).toBe("1");
    expect(within(main).getByRole<HTMLSelectElement>("combobox", { name: "Subtitles" }).value).toBe(
      "",
    );
  });

  it("labels the selectors in German", async () => {
    renderItemPage("/items/item-1", {
      fetch: itemServer([twoVersions]),
      languages: ["de"],
    });
    const main = await findMain();

    expect(await within(main).findByRole("combobox", { name: "Version" })).toBeTruthy();
    expect(within(main).getByRole("group", { name: "Wiedergabe wählen" })).toBeTruthy();
  });

  it("has no accessibility violations", async () => {
    renderItemPage("/items/item-1", { fetch: itemServer([twoVersions]) });
    const main = await findMain();
    await within(main).findByRole("combobox", { name: "Version" });

    expect(await blockingAxeViolations(main)).toEqual([]);
  });
});
