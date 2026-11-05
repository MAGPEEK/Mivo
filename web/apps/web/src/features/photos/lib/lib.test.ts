import type { ItemDetail, ItemSummary } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import {
  cameraName,
  displaySize,
  exposureSettings,
  exposureTimeText,
  formatTakenAt,
  formatTakenDate,
  toPhotoDetails,
  type PhotoFactTexts,
} from "./photoFacts";
import { toPhotoTile } from "./photoTile";

const texts: PhotoFactTexts = {
  language: "en",
  label: (key) => `label:${key}`,
  fileTime: (date) => `${date} (file date)`,
};

function photoDetail(fields: Partial<ItemDetail> = {}): ItemDetail {
  return {
    id: "photo-1",
    libraryId: "library-1",
    kind: "photo",
    title: "IMG_0101.jpg",
    artwork: { square: { id: "photo-1", tag: "abc" } },
    thumbnailState: "ready",
    files: [
      {
        id: "file-1",
        fileName: "IMG_0101.jpg",
        sizeBytes: 1_000,
        modifiedAt: "2024-07-14T09:30:00Z",
        available: true,
        streams: [],
      },
    ],
    photo: {
      takenAt: "2024-07-14T09:30:00Z",
      takenAtSource: "exif",
      make: "Lumora",
      model: "LX-100",
      lens: "Lumora 24-70mm F2.8",
      exposureTime: 0.004,
      fNumber: 5.6,
      focalLengthMm: 35,
      iso: 100,
      width: 6000,
      height: 4000,
    },
    ...fields,
  };
}

describe("formatTakenAt", () => {
  it("shows the camera's wall clock in UTC, never moved to the viewer's zone", () => {
    expect(formatTakenAt("2024-07-14T23:30:00Z", "en")).toBe("July 14, 2024 at 11:30 PM");
    expect(formatTakenAt("2024-07-14T23:30:00Z", "de")).toBe("14. Juli 2024 um 23:30");
  });

  it("gives nothing for a malformed time", () => {
    expect(formatTakenAt("yesterday", "en")).toBe("");
    expect(formatTakenDate("yesterday", "en")).toBe("");
  });

  it("formats a short date for tiles", () => {
    expect(formatTakenDate("2024-07-14T23:30:00Z", "en")).toBe("Jul 14, 2024");
  });
});

describe("cameraName", () => {
  it.each([
    { make: "Lumora", model: "LX-100", expected: "Lumora LX-100" },
    { make: "Canon", model: "Canon EOS R5", expected: "Canon EOS R5" },
    { make: "Lumora", model: undefined, expected: "Lumora" },
    { make: undefined, model: "LX-100", expected: "LX-100" },
    { make: " ", model: undefined, expected: "" },
  ])("names $make / $model as '$expected'", ({ make, model, expected }) => {
    expect(
      cameraName({
        ...(make === undefined ? {} : { make }),
        ...(model === undefined ? {} : { model }),
      }),
    ).toBe(expected);
  });
});

describe("exposure", () => {
  it.each([
    { seconds: 0.004, expected: "1/250 s" },
    { seconds: 1 / 60, expected: "1/60 s" },
    { seconds: 2, expected: "2 s" },
    { seconds: 1.5, expected: "1.5 s" },
  ])("writes $seconds s as $expected", ({ seconds, expected }) => {
    expect(exposureTimeText(seconds, "en")).toBe(expected);
  });

  it("joins the settings that are present in the UI language", () => {
    const photo = photoDetail().photo;
    if (photo === undefined) {
      throw new Error("fixture without photo");
    }
    expect(exposureSettings(photo, "en")).toBe("1/250 s · f/5.6 · 35 mm · ISO 100");
    expect(exposureSettings(photo, "de")).toBe("1/250 s · f/5,6 · 35 mm · ISO 100");
    expect(exposureSettings({ takenAt: photo.takenAt, takenAtSource: "exif" }, "en")).toBe("");
  });

  it("formats the displayed size, or nothing without both sides", () => {
    expect(
      displaySize({ takenAt: "", takenAtSource: "exif", width: 6000, height: 4000 }, "en"),
    ).toBe("6,000 × 4,000");
    expect(displaySize({ takenAt: "", takenAtSource: "exif", width: 6000 }, "en")).toBe("");
  });
});

describe("toPhotoDetails", () => {
  it("lists the facts in order with the image and its aspect ratio", () => {
    const view = toPhotoDetails(photoDetail(), texts);

    expect(view.image).toEqual({ id: "photo-1", tag: "abc" });
    expect(view.unsupported).toBe(false);
    expect(view.isVideo).toBe(false);
    expect(view.aspectRatio).toBe("6000 / 4000");
    expect(view.facts).toEqual([
      { key: "taken", label: "label:taken", value: "July 14, 2024 at 9:30 AM" },
      { key: "camera", label: "label:camera", value: "Lumora LX-100" },
      { key: "lens", label: "label:lens", value: "Lumora 24-70mm F2.8" },
      { key: "exposure", label: "label:exposure", value: "1/250 s · f/5.6 · 35 mm · ISO 100" },
      { key: "size", label: "label:size", value: "6,000 × 4,000" },
      { key: "file", label: "label:file", value: "IMG_0101.jpg" },
    ]);
  });

  it("marks a capture time that is only the file's time and drops missing facts", () => {
    const view = toPhotoDetails(
      photoDetail({
        files: [],
        photo: { takenAt: "2024-07-14T09:30:00Z", takenAtSource: "file" },
      }),
      texts,
    );

    expect(view.aspectRatio).toBeUndefined();
    expect(view.facts).toEqual([
      { key: "taken", label: "label:taken", value: "July 14, 2024 at 9:30 AM (file date)" },
    ]);
  });

  it("requests no image for an unsupported photo or a video", () => {
    const unsupported = toPhotoDetails(photoDetail({ thumbnailState: "unsupported" }), texts);
    expect(unsupported.image).toBeUndefined();
    expect(unsupported.unsupported).toBe(true);

    const video = toPhotoDetails(photoDetail({ playItemId: "photo-1" }), texts);
    expect(video.image).toBeUndefined();
    expect(video.isVideo).toBe(true);
  });

  it("shows only the file name of an item without an EXIF block", () => {
    const withoutPhoto = photoDetail();
    delete withoutPhoto.photo;
    expect(toPhotoDetails(withoutPhoto, texts).facts.map((fact) => fact.key)).toEqual(["file"]);
  });
});

describe("toPhotoTile", () => {
  const undated: ItemSummary = {
    id: "photo-1",
    libraryId: "library-1",
    kind: "photo",
    title: "IMG_0101.jpg",
    thumbnailState: "ready",
    square: { id: "photo-1", tag: "abc" },
  };
  const summary: ItemSummary = { ...undated, takenAt: "2024-07-14T09:30:00Z" };

  it("names the photo with its capture date and keeps its tile image", () => {
    expect(toPhotoTile(summary, "en")).toEqual({
      id: "photo-1",
      label: "IMG_0101.jpg, Jul 14, 2024",
      image: { id: "photo-1", tag: "abc" },
      unsupported: false,
      durationSeconds: undefined,
    });
  });

  it("drops the image of an unsupported photo and keeps the length of a video", () => {
    expect(toPhotoTile({ ...undated, thumbnailState: "unsupported" }, "en")).toMatchObject({
      label: "IMG_0101.jpg",
      image: undefined,
      unsupported: true,
    });
    expect(
      toPhotoTile({ ...summary, media: { available: true, durationSeconds: 3 } }, "en"),
    ).toMatchObject({ durationSeconds: 3 });
  });
});
