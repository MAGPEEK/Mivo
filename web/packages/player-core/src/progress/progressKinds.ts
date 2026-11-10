/** Item kinds with a watch state in 0.1.0 (ADR 0032 section 8); music and photos have none. */
export const progressKinds: readonly string[] = ["movie", "episode", "audiobook"];

/** Whether a player reports the position of an item of `kind` to the server. */
export function reportsProgress(kind: string): boolean {
  return progressKinds.includes(kind);
}
