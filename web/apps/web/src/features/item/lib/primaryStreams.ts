import type { MediaFile, MediaStream } from "@mivo/api-client";

/** The video stream playback uses: the first one that is not cover art. */
export function primaryVideoStream(file: MediaFile): MediaStream | undefined {
  return file.streams.find((stream) => stream.kind === "video" && !stream.attachedPicture);
}

/**
 * The audio stream a player starts with: the one with the default disposition, otherwise the
 * first audio stream, like ffmpeg's own stream selection.
 */
export function primaryAudioStream(file: MediaFile): MediaStream | undefined {
  const audioStreams = streamsOfKind(file, "audio");
  return audioStreams.find((stream) => stream.default) ?? audioStreams[0];
}

/** Streams of one kind in container order. Unknown kinds of later API versions never match. */
export function streamsOfKind(file: MediaFile, kind: MediaStream["kind"]): MediaStream[] {
  return file.streams.filter((stream) => stream.kind === kind);
}
