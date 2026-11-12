import type { ItemView, VersionView } from "./itemView";
import type { ResolvedChoice } from "./playChoice";

/**
 * The item as the chosen version shows it: its chips, file and track lists, and Play opening
 * that file with the picked tracks. A page without a choice (no version, or Play disabled
 * because the primary file cannot be played) stays as it is.
 */
export function withChosenVersion(
  item: ItemView,
  resolved: ResolvedChoice<VersionView> | undefined,
): ItemView {
  if (resolved === undefined || item.play.kind !== "playable") {
    return item;
  }
  const { version, choice } = resolved;
  return {
    ...item,
    facts: [...item.ownFacts, ...version.facts],
    chips: version.chips,
    file: version.file,
    audioTracks: version.audioTracks,
    subtitleTracks: version.subtitleTracks,
    play: { ...item.play, fileId: version.fileId, choice },
  };
}
