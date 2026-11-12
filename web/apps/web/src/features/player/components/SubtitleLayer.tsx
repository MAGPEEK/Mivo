import { subtitleSizeClass, type SubtitleSize } from "../lib/subtitleSizePreference";

/** Props of {@link SubtitleLayer}. */
export interface SubtitleLayerProps {
  /** The cue text to show; lines are separated by line breaks. Empty shows nothing. */
  readonly text: string;
  readonly size: SubtitleSize;
  /** Whether the control bar is visible, which lifts the text above it. */
  readonly controlsVisible: boolean;
}

/**
 * The subtitle the player draws itself (PL-12, PL-13): plain text over the lower part of the
 * video, in the size chosen for this device. Cue styling (ASS) is not drawn (task M4-19). The
 * text is a text node, never markup. Screen readers skip it: the spoken audio is the same.
 */
export function SubtitleLayer({ text, size, controlsVisible }: SubtitleLayerProps) {
  if (text === "") {
    return null;
  }
  return (
    <div
      aria-hidden="true"
      data-testid="subtitle-layer"
      className={`pointer-events-none absolute inset-x-0 flex justify-center px-6 transition-[bottom] duration-300 motion-reduce:transition-none ${controlsVisible ? "bottom-36" : "bottom-10"}`}
    >
      <p
        className={`max-w-4xl rounded-md bg-background/70 px-3 py-1 text-center font-medium whitespace-pre-line text-text ${subtitleSizeClass(size)}`}
      >
        {text}
      </p>
    </div>
  );
}
