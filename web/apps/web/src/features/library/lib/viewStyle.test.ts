import { imageWidths } from "@mivo/api-client";
import { describe, expect, it } from "vitest";

import {
  cardSize,
  cardStepCount,
  cardStepStorageKey,
  clampCardStep,
  defaultCardStep,
  imageWidthFor,
  readCardStep,
  readViewStyle,
  viewStyleStorageKey,
} from "./viewStyle";

function storageWith(entries: Record<string, string>): Storage {
  const map = new Map(Object.entries(entries));
  return { getItem: (key: string) => map.get(key) ?? null } as unknown as Storage;
}

describe("cardSize", () => {
  it.each([
    { step: 0, columnWidth: 120, sizes: "120px", imageWidth: 320 },
    { step: 1, columnWidth: 140, sizes: "140px", imageWidth: 320 },
    { step: 2, columnWidth: 160, sizes: "160px", imageWidth: 320 },
    { step: 3, columnWidth: 190, sizes: "190px", imageWidth: 480 },
    { step: 4, columnWidth: 230, sizes: "230px", imageWidth: 480 },
  ])("maps step $step to $columnWidth px and image width $imageWidth", (expected) => {
    expect(cardSize(expected.step)).toEqual({
      columnWidth: expected.columnWidth,
      sizes: expected.sizes,
      imageWidth: expected.imageWidth,
    });
  });

  it("only ever asks for an allowed image width", () => {
    for (let step = 0; step < cardStepCount; step += 1) {
      for (const format of ["poster", "episode", "square"] as const) {
        expect(imageWidths).toContain(cardSize(step, format).imageWidth);
      }
    }
  });

  it("makes episode columns wider while poster and square keep the poster width", () => {
    expect(cardSize(2, "episode").columnWidth).toBe(256);
    expect(cardSize(2, "square").columnWidth).toBe(160);
  });

  it("clamps out-of-range and invalid steps", () => {
    expect(clampCardStep(-3)).toBe(0);
    expect(clampCardStep(9)).toBe(cardStepCount - 1);
    expect(clampCardStep(Number.NaN)).toBe(defaultCardStep);
  });

  it("falls back to the largest rendition for huge widths", () => {
    expect(imageWidthFor(99999)).toBe(1920);
  });
});

describe("stored preference", () => {
  it("reads valid values", () => {
    const storage = storageWith({ [viewStyleStorageKey]: "details", [cardStepStorageKey]: "4" });
    expect(readViewStyle(storage)).toBe("details");
    expect(readCardStep(storage)).toBe(4);
  });

  it("falls back to the defaults for unknown values and a missing storage", () => {
    const storage = storageWith({ [viewStyleStorageKey]: "cover", [cardStepStorageKey]: "7" });
    expect(readViewStyle(storage)).toBe("grid");
    expect(readCardStep(storage)).toBe(defaultCardStep);
    expect(readViewStyle(undefined)).toBe("grid");
    expect(readCardStep(undefined)).toBe(defaultCardStep);
  });
});
