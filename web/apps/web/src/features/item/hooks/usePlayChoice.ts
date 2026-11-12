import { useMemo, useState } from "react";

import { browserLocalStorage } from "../../../shared/lib/persistentFlag";
import {
  chooseAudio,
  chooseSubtitle,
  chooseVersion,
  resolveChoice,
  type ResolvedChoice,
  type StoredChoice,
} from "../lib/playChoice";
import { readStoredChoice, writeStoredChoice } from "../lib/playChoiceStorage";
import type { VersionView } from "../lib/itemView";
import type { ChoiceOption } from "../lib/versions";

/** What the selectors show and do. */
export interface PlayChoiceView {
  /** The shown version and tracks; `undefined` when the item has no playable version. */
  readonly resolved: ResolvedChoice<VersionView> | undefined;
  /** One entry per version, labelled for the version picker. */
  readonly versionOptions: readonly ChoiceOption[];
  readonly pickVersion: (fileId: string) => void;
  readonly pickAudio: (value: string) => void;
  /** Picks a subtitle track by id, or "Off" with the empty string. */
  readonly pickSubtitle: (value: string) => void;
}

interface Picked {
  readonly itemId: string;
  readonly choice: StoredChoice;
}

/**
 * The version, audio and subtitle choice of the detail page (DT-07). It starts from what was
 * remembered for the item on this device (or the defaults) and remembers every pick. Tracks
 * the viewer did not pick stay unset, so the server's default policy still decides them.
 */
export function usePlayChoice(itemId: string, versions: readonly VersionView[]): PlayChoiceView {
  const remembered = useMemo(() => readStoredChoice(browserLocalStorage(), itemId), [itemId]);
  // Keyed by item: the page is reused when the viewer goes to the next episode.
  const [picked, setPicked] = useState<Picked>();
  const stored = picked?.itemId === itemId ? picked.choice : remembered;
  const resolved = resolveChoice(versions, stored);
  const remember = (choice: StoredChoice) => {
    setPicked({ itemId, choice });
    writeStoredChoice(browserLocalStorage(), itemId, choice);
  };
  const fileId = resolved?.version.fileId ?? "";
  return {
    resolved,
    versionOptions: versions.map((version) => ({ value: version.fileId, label: version.label })),
    pickVersion: (next) => {
      remember(chooseVersion(next));
    },
    pickAudio: (value) => {
      remember(chooseAudio(stored, fileId, value));
    },
    pickSubtitle: (value) => {
      remember(chooseSubtitle(stored, fileId, value));
    },
  };
}
