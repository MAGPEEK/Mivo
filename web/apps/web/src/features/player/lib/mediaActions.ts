// The control bar's actions on the media element. Seeking is not among them: it goes through
// the engine, which decides between a local seek and a new session.

/** The part of a media element the actions change. */
export interface ControllableMedia {
  readonly paused: boolean;
  muted: boolean;
  volume: number;
  play(): Promise<void>;
  pause(): void;
}

/** Plays a paused element and pauses a playing one. */
export function togglePlayback(media: ControllableMedia): void {
  if (!media.paused) {
    media.pause();
    return;
  }
  // Refused without a user gesture or without a source; the element then stays paused and the
  // button on "Play". jsdom's play() returns nothing, hence the wrapper.
  Promise.resolve()
    .then(() => media.play())
    .catch(() => undefined);
}

/** Mutes or unmutes the element, keeping its volume. */
export function toggleMute(media: ControllableMedia): void {
  media.muted = !media.muted;
}

/** Sets the volume (0 to 1); 0 mutes, anything above unmutes. */
export function setVolume(media: ControllableMedia, volume: number): void {
  media.volume = volume;
  media.muted = volume === 0;
}
