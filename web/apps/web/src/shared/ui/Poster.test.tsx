import type { ImageRef } from "@mivo/api-client";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../test/axe";
import { Poster } from "./Poster";

const poster: ImageRef = { id: "poster-1", tag: "tag-1" };
const otherPoster: ImageRef = { id: "poster-2", tag: "tag-2" };
const sizes = "(min-width: 1280px) 12vw, 50vw";

function placeholder(container: HTMLElement): Element | null {
  return container.querySelector("[data-poster-placeholder]");
}

describe("Poster", () => {
  it("loads the image lazily in every width from 160 to 960", async () => {
    const { container } = render(
      <Poster imageRef={poster} aspect="poster" alt="Big Buck Bunny" sizes={sizes} />,
    );

    const image = screen.getByRole("img", { name: "Big Buck Bunny" });
    expect(image.getAttribute("src")).toBe("/api/v1/images/poster-1?width=320&tag=tag-1");
    expect(image.getAttribute("srcset")).toBe(
      [160, 320, 480, 640, 960]
        .map(
          (width) => `/api/v1/images/poster-1?width=${String(width)}&tag=tag-1 ${String(width)}w`,
        )
        .join(", "),
    );
    expect(image.getAttribute("sizes")).toBe(sizes);
    expect(image.getAttribute("loading")).toBe("lazy");
    expect(image.getAttribute("decoding")).toBe("async");
    expect(placeholder(container)).toBeNull();
    expect(await blockingAxeViolations(container)).toEqual([]);
  });

  it.each([
    { aspect: "poster", className: "aspect-2/3" },
    { aspect: "still", className: "aspect-video" },
  ] as const)("keeps a fixed $aspect box", ({ aspect, className }) => {
    const { container } = render(<Poster aspect={aspect} alt="Clip" sizes={sizes} />);

    expect(container.firstElementChild?.classList.contains(className)).toBe(true);
  });

  it("shows the placeholder with the title as its name without an image", async () => {
    const { container } = render(
      <Poster
        aspect="poster"
        alt="Big Buck Bunny"
        sizes={sizes}
        icon={<svg data-testid="kind" />}
      />,
    );

    expect(screen.getByRole("img", { name: "Big Buck Bunny" }).tagName).not.toBe("IMG");
    expect(placeholder(container)).not.toBeNull();
    expect(screen.getByTestId("kind")).toBeTruthy();
    expect(await blockingAxeViolations(container)).toEqual([]);
  });

  it("falls back to the placeholder when the image fails to load", () => {
    const { container } = render(
      <Poster imageRef={poster} aspect="poster" alt="Big Buck Bunny" sizes={sizes} />,
    );

    fireEvent.error(screen.getByRole("img", { name: "Big Buck Bunny" }));

    expect(container.querySelector("img")).toBeNull();
    expect(placeholder(container)).not.toBeNull();
  });

  it("tries again when a failed image is replaced by another one", () => {
    const { container, rerender } = render(
      <Poster imageRef={poster} aspect="poster" alt="Big Buck Bunny" sizes={sizes} />,
    );
    fireEvent.error(screen.getByRole("img", { name: "Big Buck Bunny" }));

    rerender(<Poster imageRef={otherPoster} aspect="poster" alt="Big Buck Bunny" sizes={sizes} />);

    expect(container.querySelector("img")?.getAttribute("src")).toContain("poster-2");
  });

  it("tries a replaced image with the same id but a new tag again", () => {
    const { container, rerender } = render(
      <Poster imageRef={poster} aspect="poster" alt="Clip" sizes={sizes} />,
    );
    fireEvent.error(screen.getByRole("img", { name: "Clip" }));

    rerender(
      <Poster imageRef={{ ...poster, tag: "tag-new" }} aspect="poster" alt="Clip" sizes={sizes} />,
    );

    expect(container.querySelector("img")?.getAttribute("src")).toContain("tag=tag-new");
  });

  it("stays on the placeholder for an equal reference that already failed", () => {
    const { container, rerender } = render(
      <Poster imageRef={poster} aspect="poster" alt="Clip" sizes={sizes} />,
    );
    fireEvent.error(screen.getByRole("img", { name: "Clip" }));

    rerender(<Poster imageRef={{ ...poster }} aspect="poster" alt="Clip" sizes={sizes} />);

    expect(container.querySelector("img")).toBeNull();
  });

  it("is decorative with an empty alt: empty image alt, hidden placeholder", async () => {
    const { container, rerender } = render(
      <Poster imageRef={poster} aspect="poster" alt="" sizes={sizes} />,
    );

    expect(container.querySelector("img")?.getAttribute("alt")).toBe("");
    expect(screen.queryByRole("img")).toBeNull();

    rerender(<Poster aspect="poster" alt="" sizes={sizes} />);

    expect(placeholder(container)?.getAttribute("aria-hidden")).toBe("true");
    expect(placeholder(container)?.hasAttribute("role")).toBe(false);
    expect(await blockingAxeViolations(container)).toEqual([]);
  });
});
