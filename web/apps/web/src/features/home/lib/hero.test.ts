import {
  colors,
  compositeColor,
  contrastRatio,
  textScrimOpacity,
  type ColorToken,
} from "@mivo/ui-tokens";
import { afterEach, describe, expect, it } from "vitest";

import { heroMovie, heroShow } from "../test/homeFixtures";
import { heroMetaParts, heroRuntime, type HeroMetaTexts, type HeroRuntime } from "./heroMeta";
import { heroEnabledStorageKey, readHeroEnabled, writeHeroEnabled } from "./heroPreference";
import { heroScrimClass, heroTextPanelClass } from "./heroScrim";

describe("heroRuntime", () => {
  it.each<[string, number | undefined, HeroRuntime | undefined]>([
    ["unknown", undefined, undefined],
    ["zero", 0, undefined],
    ["not finite", Number.NaN, undefined],
    ["a short clip counts as a minute", 20, { unit: "minutes", minutes: 1 }],
    ["minutes, rounded", 888, { unit: "minutes", minutes: 15 }],
    ["exactly an hour", 3600, { unit: "hoursMinutes", hours: 1, minutes: 0 }],
    ["hours and minutes", 5423, { unit: "hoursMinutes", hours: 1, minutes: 30 }],
  ])("%s", (_, seconds, expected) => {
    expect(heroRuntime(seconds)).toEqual(expected);
  });
});

describe("heroMetaParts", () => {
  const texts: HeroMetaTexts = {
    runtime: (runtime) =>
      runtime.unit === "minutes"
        ? `${String(runtime.minutes)} min`
        : `${String(runtime.hours)} h ${String(runtime.minutes)} min`,
    genres: (genres) => genres.join(", "),
  };

  it("lists year, runtime, genres and resolution of a movie", () => {
    expect(heroMetaParts(heroMovie, texts)).toEqual([
      "2010",
      "15 min",
      "Animation, Fantasy",
      "1080p",
    ]);
  });

  it("leaves out what a show does not know", () => {
    expect(heroMetaParts(heroShow, texts)).toEqual([]);
    expect(heroMetaParts({ ...heroShow, year: 2015, genres: [] }, texts)).toEqual(["2015"]);
  });
});

describe("hero scrim", () => {
  // WCAG AA for normal text; the hero text sits on the scrim over an arbitrary backdrop.
  const textContrast = 4.5;
  const percent = 100;
  const textTokens: readonly ColorToken[] = ["text", "textMuted"];
  // The brightest token colours stand in for a bright backdrop; ui-tokens also checks white.
  const brightBackdrops: readonly ColorToken[] = ["text", "accentHover", "transcode"];

  it.each(
    textTokens.flatMap((token) => brightBackdrops.map((backdrop) => [token, backdrop] as const)),
  )("keeps %s readable on a %s backdrop", (token, backdrop) => {
    const scrim = compositeColor(colors.background, textScrimOpacity, colors[backdrop]);
    expect(contrastRatio(colors[token], scrim)).toBeGreaterThanOrEqual(textContrast);
  });

  it("uses the opacity of the text scrim token in its classes", () => {
    const opacity = String(Math.round(textScrimOpacity * percent));
    expect(heroScrimClass).toContain(`from-background/${opacity} `);
    expect(heroScrimClass).toContain(`via-background/${opacity} `);
    // Below sm the whole text panel is solid scrim, not a fading gradient.
    expect(heroTextPanelClass).toContain(`max-sm:bg-background/${opacity} `);
  });

  it("uses no other opacity of the background colour", () => {
    const opacity = String(Math.round(textScrimOpacity * percent));
    const opacities = `${heroScrimClass} ${heroTextPanelClass}`.match(/background\/\d+/g) ?? [];
    expect(opacities.length).toBeGreaterThan(0);
    expect(new Set(opacities)).toEqual(new Set([`background/${opacity}`]));
  });
});

describe("hero preference", () => {
  afterEach(() => {
    window.localStorage.clear();
  });

  it("is on by default", () => {
    expect(readHeroEnabled()).toBe(true);
  });

  it("remembers the choice under hero.enabled", () => {
    writeHeroEnabled(false);

    expect(heroEnabledStorageKey).toMatch(/\.hero\.enabled$/);
    expect(window.localStorage.getItem(heroEnabledStorageKey)).toBe("false");
    expect(readHeroEnabled()).toBe(false);
  });
});
