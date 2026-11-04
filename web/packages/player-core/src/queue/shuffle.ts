const randomRange = 4294967296;
const mulberryIncrement = 0x6d2b79f5;

/** Small seeded PRNG (mulberry32): same seed, same sequence, in every runtime. */
export function seededRandom(seed: number): () => number {
  let state = seed >>> 0;
  return () => {
    state = (state + mulberryIncrement) >>> 0;
    let mixed = Math.imul(state ^ (state >>> 15), state | 1);
    mixed ^= mixed + Math.imul(mixed ^ (mixed >>> 7), mixed | 61);
    return ((mixed ^ (mixed >>> 14)) >>> 0) / randomRange;
  };
}

/** Fisher-Yates shuffle returning a new array; deterministic for a seed. */
export function shuffled<T>(values: readonly T[], seed: number): T[] {
  const result = [...values];
  const random = seededRandom(seed);
  for (let index = result.length - 1; index > 0; index -= 1) {
    const other = Math.floor(random() * (index + 1));
    [result[index], result[other]] = [result[other] as T, result[index] as T];
  }
  return result;
}
