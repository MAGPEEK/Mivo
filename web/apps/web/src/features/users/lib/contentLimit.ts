import type { AgeRating, ContentLimit, ContentLimitPreset } from "@mivo/api-client";

/** The presets the dialog offers, in display order (US-11). */
export const contentLimitPresets = [
  "youngChildren",
  "children",
  "teens",
  "adults",
  "custom",
] as const satisfies readonly ContentLimitPreset[];

/** A preset this client knows; servers may send more (ADR 0017). */
export type KnownPreset = (typeof contentLimitPresets)[number];

/** The age buckets "Custom" offers (ADR 0032 §7). */
export const customAges = [0, 6, 12, 16, 18] as const satisfies readonly AgeRating[];

/** An age bucket this client knows. */
export type KnownAge = (typeof customAges)[number];

// ADR 0032 §7: the fixed presets map to these ages; "adults" has no limit.
const presetAges: Readonly<Record<Exclude<KnownPreset, "custom">, KnownAge | null>> = {
  youngChildren: 0,
  children: 6,
  teens: 12,
  adults: null,
};

/** Age "Custom" starts with: the one bucket no fixed preset covers below adults. */
const defaultCustomAge: KnownAge = 16;

/** The content limit as the dialog edits it. */
export interface ContentLimitDraft {
  readonly preset: KnownPreset;
  /** The age of "Custom"; kept while another preset is chosen, so switching back restores it. */
  readonly customAge: KnownAge;
  readonly hideUnrated: boolean;
}

/** No limit: what a new account gets unless the admin chooses otherwise. */
export const noContentLimit: ContentLimitDraft = {
  preset: "adults",
  customAge: defaultCustomAge,
  hideUnrated: false,
};

function isKnownAge(age: number): age is KnownAge {
  return (customAges as readonly number[]).includes(age);
}

function presetForAge(maxAge: number | null): KnownPreset {
  const fixed = Object.entries(presetAges).find(([, age]) => age === maxAge);
  return fixed === undefined ? "custom" : (fixed[0] as KnownPreset);
}

/**
 * The draft of a stored limit. The preset is derived from `maxAge` again rather than read,
 * so an unknown preset or age of a newer server still lands on a sensible choice.
 */
export function contentLimitDraft(limit: ContentLimit): ContentLimitDraft {
  const age = limit.maxAge;
  const knownAge = age !== null && isKnownAge(age) ? age : undefined;
  const unknownAge = age !== null && knownAge === undefined;
  return {
    preset: unknownAge ? "custom" : presetForAge(age),
    customAge: knownAge ?? defaultCustomAge,
    hideUnrated: limit.hideUnrated,
  };
}

/** The highest age the draft allows; null for no limit. */
export function draftMaxAge(draft: ContentLimitDraft): KnownAge | null {
  return draft.preset === "custom" ? draft.customAge : presetAges[draft.preset];
}

/** The request body of a draft; the preset always matches `maxAge` as the contract asks. */
export function toContentLimit(draft: ContentLimitDraft): ContentLimit {
  return { preset: draft.preset, maxAge: draftMaxAge(draft), hideUnrated: draft.hideUnrated };
}

/**
 * Chooses another preset. Moving between "no limit" and a limit also flips "Hide unrated
 * titles", because clients default it to on for limited profiles (contract) and an unlimited
 * profile has nothing to hide.
 */
export function withPreset(draft: ContentLimitDraft, preset: KnownPreset): ContentLimitDraft {
  const wasLimited = draftMaxAge(draft) !== null;
  const next = { ...draft, preset };
  const limited = draftMaxAge(next) !== null;
  return wasLimited === limited ? next : { ...next, hideUnrated: limited };
}

/** Whether two drafts send the same limit. */
export function sameContentLimit(left: ContentLimitDraft, right: ContentLimitDraft): boolean {
  return draftMaxAge(left) === draftMaxAge(right) && left.hideUnrated === right.hideUnrated;
}
