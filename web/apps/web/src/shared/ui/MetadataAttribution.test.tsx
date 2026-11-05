import { render, screen } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it } from "vitest";

import { createI18n } from "../i18n";
import { MetadataAttribution, creditedMetadataSources } from "./MetadataAttribution";

const disclaimer = "This product uses the TMDB API but is not endorsed or certified by TMDB.";

function renderAttribution(sources: readonly string[]) {
  return render(
    <I18nextProvider i18n={createI18n(["en"])}>
      <MetadataAttribution sources={sources} />
    </I18nextProvider>,
  );
}

describe("MetadataAttribution", () => {
  it("credits TMDB with the bundled logo and the disclaimer", () => {
    renderAttribution(["nfo", "tmdb"]);

    const logo = screen.getByRole("img", { name: "TMDB" });
    expect(logo.getAttribute("src")).not.toMatch(/^(https?:)?\/\//);
    expect(logo.getAttribute("src")).toContain("tmdb-logo");
    expect(screen.getByText("Powered by")).toBeTruthy();
    expect(screen.getByText(disclaimer)).toBeTruthy();
    expect(screen.queryByText(/MusicBrainz/)).toBeNull();
  });

  it("credits MusicBrainz and the Cover Art Archive for music", () => {
    renderAttribution(["musicbrainz", "coverartarchive"]);

    expect(screen.getByText("Music metadata provided by MusicBrainz (CC0).")).toBeTruthy();
    expect(screen.getByText(/Cover Art Archive/)).toBeTruthy();
    expect(screen.queryByRole("img")).toBeNull();
  });

  it("renders nothing for sources without a required credit", () => {
    const { container } = renderAttribution(["nfo", "embedded"]);

    expect(container.innerHTML).toBe("");
  });

  it("lists every credited source", () => {
    renderAttribution(creditedMetadataSources);

    expect(screen.getByText(disclaimer)).toBeTruthy();
    expect(screen.getByText(/MusicBrainz/)).toBeTruthy();
    expect(screen.getByText(/Cover Art Archive/)).toBeTruthy();
  });
});
