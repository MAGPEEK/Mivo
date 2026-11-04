import { describe, expect, expectTypeOf, it } from "vitest";

import { imageSrcSet, imageUrl, imageWidths, type ImageRef, type ImageWidth } from "./index";

const poster: ImageRef = { id: "7e6d5c4b-3a2f-4e1d-9c0b-a9f8e7d6c5b4", tag: "9f86d081884c7d65" };

describe("imageUrl", () => {
  it("builds the root-relative image URL with width and tag", () => {
    expect(imageUrl(poster, 320)).toBe(
      "/api/v1/images/7e6d5c4b-3a2f-4e1d-9c0b-a9f8e7d6c5b4?width=320&tag=9f86d081884c7d65",
    );
  });

  it("omits an empty tag, so the URL never claims a cacheable version", () => {
    expect(imageUrl({ id: "id", tag: "" }, 160)).toBe("/api/v1/images/id?width=160");
  });

  it.each<{ name: string; ref: ImageRef; expected: string }>([
    {
      name: "path characters in the id",
      ref: { id: "../a/b?c#d", tag: "abc" },
      expected: "/api/v1/images/..%2Fa%2Fb%3Fc%23d?width=160&tag=abc",
    },
    {
      name: "query characters in the tag",
      ref: { id: "id", tag: "a&width=1920 b+c" },
      expected: "/api/v1/images/id?width=160&tag=a%26width%3D1920%20b%2Bc",
    },
    {
      name: "non-ASCII text",
      ref: { id: "bild-ä", tag: "ü" },
      expected: "/api/v1/images/bild-%C3%A4?width=160&tag=%C3%BC",
    },
  ])("URL-encodes $name", ({ ref, expected }) => {
    expect(imageUrl(ref, 160)).toBe(expected);
  });

  it("accepts only the widths of the contract", () => {
    expectTypeOf(imageUrl).parameter(1).toEqualTypeOf<ImageWidth>();
    expectTypeOf<ImageWidth>().toEqualTypeOf<160 | 320 | 480 | 640 | 960 | 1280 | 1920>();

    // @ts-expect-error 300 is not an ImageWidth, so the server would reject it.
    imageUrl(poster, 300);
  });
});

describe("imageWidths", () => {
  it("lists every width of the contract in ascending order", () => {
    expectTypeOf<(typeof imageWidths)[number]>().toEqualTypeOf<ImageWidth>();
    expect([...imageWidths]).toEqual([160, 320, 480, 640, 960, 1280, 1920]);
  });
});

describe("imageSrcSet", () => {
  it("offers each width once, ascending, with its w descriptor", () => {
    const base = "/api/v1/images/7e6d5c4b-3a2f-4e1d-9c0b-a9f8e7d6c5b4";

    expect(imageSrcSet(poster, [640, 320, 640])).toBe(
      `${base}?width=320&tag=9f86d081884c7d65 320w, ${base}?width=640&tag=9f86d081884c7d65 640w`,
    );
  });

  it("encodes commas and spaces, which would split a srcset candidate", () => {
    expect(imageSrcSet({ id: "a, b", tag: "c d" }, [160])).toBe(
      "/api/v1/images/a%2C%20b?width=160&tag=c%20d 160w",
    );
  });

  it("is empty without widths", () => {
    expect(imageSrcSet(poster, [])).toBe("");
  });
});
