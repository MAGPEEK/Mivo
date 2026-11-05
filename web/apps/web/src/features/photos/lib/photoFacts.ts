import type { ImageRef, ItemDetail, ItemPhoto } from "@mivo/api-client";

/** One labelled fact of the photo info list, e.g. "Camera" and "Lumora LX-100". */
export interface PhotoFact {
  /** Stable key of the fact, also the translation key of its label. */
  readonly key: PhotoFactKey;
  readonly label: string;
  readonly value: string;
}

/** The facts the photo page can show, in display order. */
export type PhotoFactKey = "taken" | "camera" | "lens" | "exposure" | "size" | "file";

/** Everything the photo page shows of a loaded photo, as display text. */
export interface PhotoDetailsView {
  /** The photo as an image reference; undefined when the server cannot render it. */
  readonly image: ImageRef | undefined;
  /** The server knows it cannot render the photo (thumbnailState "unsupported"). */
  readonly unsupported: boolean;
  /** A video of the photo library: the page offers Play instead of a still image. */
  readonly isVideo: boolean;
  /** CSS aspect ratio of the upright photo, e.g. `6000 / 4000`; undefined when unknown. */
  readonly aspectRatio: string | undefined;
  readonly facts: readonly PhotoFact[];
}

/** The translated texts {@link toPhotoDetails} needs; the hook builds them from i18next. */
export interface PhotoFactTexts {
  /** The UI language, for date and number formats, e.g. `de`. */
  readonly language: string;
  readonly label: (key: PhotoFactKey) => string;
  /** Marks a capture time that is only the file's modification time. */
  readonly fileTime: (date: string) => string;
}

const factSeparator = " · ";
// Exposure times below one second read as the shutter fraction photographers know (1/250 s).
const fractionLimitSeconds = 1;
const oneDecimal = { maximumFractionDigits: 1 } as const;

/**
 * A capture time as date and time. The API sends the camera's wall-clock reading with offset Z,
 * so it is formatted in UTC and never moved to the viewer's time zone (ItemPhoto.takenAt).
 */
export function formatTakenAt(takenAt: string, language: string): string {
  const date = new Date(takenAt);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat(language, {
    dateStyle: "long",
    timeStyle: "short",
    timeZone: "UTC",
  }).format(date);
}

/** A capture time as a short date for tile labels, in UTC like {@link formatTakenAt}. */
export function formatTakenDate(takenAt: string, language: string): string {
  const date = new Date(takenAt);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeZone: "UTC" }).format(date);
}

/** Make and model, without repeating the make when the model already starts with it. */
export function cameraName(photo: Pick<ItemPhoto, "make" | "model">): string {
  const make = photo.make?.trim() ?? "";
  const model = photo.model?.trim() ?? "";
  if (make === "" || model.toLowerCase().startsWith(make.toLowerCase())) {
    return model;
  }
  return model === "" ? make : `${make} ${model}`;
}

/** An exposure time as photographers write it: `1/250 s`, `2 s`. */
export function exposureTimeText(seconds: number, language: string): string {
  if (seconds < fractionLimitSeconds) {
    return `1/${String(Math.round(1 / seconds))} s`;
  }
  return `${new Intl.NumberFormat(language, oneDecimal).format(seconds)} s`;
}

/** Exposure time, aperture, focal length and ISO, e.g. `1/250 s · f/5.6 · 35 mm · ISO 100`. */
export function exposureSettings(photo: ItemPhoto, language: string): string {
  const number = new Intl.NumberFormat(language, oneDecimal);
  return [
    photo.exposureTime === undefined ? "" : exposureTimeText(photo.exposureTime, language),
    photo.fNumber === undefined ? "" : `f/${number.format(photo.fNumber)}`,
    photo.focalLengthMm === undefined ? "" : `${number.format(photo.focalLengthMm)} mm`,
    photo.iso === undefined ? "" : `ISO ${String(photo.iso)}`,
  ]
    .filter((part) => part !== "")
    .join(factSeparator);
}

/** The displayed size in pixels, e.g. `6,000 × 4,000`; empty when unknown. */
export function displaySize(photo: ItemPhoto, language: string): string {
  if (photo.width === undefined || photo.height === undefined) {
    return "";
  }
  const number = new Intl.NumberFormat(language);
  return `${number.format(photo.width)} × ${number.format(photo.height)}`;
}

function takenText(photo: ItemPhoto, texts: PhotoFactTexts): string {
  const taken = formatTakenAt(photo.takenAt, texts.language);
  return taken !== "" && photo.takenAtSource === "file" ? texts.fileTime(taken) : taken;
}

function photoFacts(item: ItemDetail, texts: PhotoFactTexts): PhotoFact[] {
  const photo = item.photo;
  const values: Readonly<Record<PhotoFactKey, string>> = {
    taken: photo === undefined ? "" : takenText(photo, texts),
    camera: photo === undefined ? "" : cameraName(photo),
    lens: photo?.lens?.trim() ?? "",
    exposure: photo === undefined ? "" : exposureSettings(photo, texts.language),
    size: photo === undefined ? "" : displaySize(photo, texts.language),
    file: item.files[0]?.fileName ?? "",
  };
  const order: readonly PhotoFactKey[] = ["taken", "camera", "lens", "exposure", "size", "file"];
  return order
    .filter((key) => values[key] !== "")
    .map((key) => ({ key, label: texts.label(key), value: values[key] }));
}

/** Turns a photo item into what the photo page shows: the image, its shape and its facts. */
export function toPhotoDetails(item: ItemDetail, texts: PhotoFactTexts): PhotoDetailsView {
  const width = item.photo?.width;
  const height = item.photo?.height;
  const unsupported = item.thumbnailState === "unsupported";
  // The API names something to play only for a photo that is a video (ItemDetail.playItemId).
  const isVideo = item.playItemId !== undefined;
  return {
    image: unsupported || isVideo ? undefined : item.artwork?.square,
    unsupported,
    isVideo,
    aspectRatio:
      width === undefined || height === undefined
        ? undefined
        : `${String(width)} / ${String(height)}`,
    facts: photoFacts(item, texts),
  };
}
