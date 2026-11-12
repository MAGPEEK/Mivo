import { streamInfo, type PlaybackSession } from "@mivo/player-core";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { I18nextProvider } from "react-i18next";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createAppI18n } from "../../../app/i18n";
import { blockingAxeViolations } from "../../../test/axe";
import { directPlaySession, remuxSession, transcodeSession } from "../test/playerFixtures";
import { ModeBadge } from "./ModeBadge";

afterEach(cleanup);

function renderBadge(session: PlaybackSession, language = "en", open = false) {
  const onToggle = vi.fn();
  const view = (next: PlaybackSession) => (
    <I18nextProvider i18n={createAppI18n([language])}>
      <ModeBadge
        info={streamInfo(next)}
        open={open}
        onToggle={onToggle}
        buttonRef={createRef<HTMLButtonElement>()}
      />
    </I18nextProvider>
  );
  const rendered = render(view(session));
  const rerenderWith = (next: PlaybackSession) => {
    rendered.rerender(view(next));
  };
  return { onToggle, rerenderWith };
}

describe("ModeBadge", () => {
  it.each([
    { session: directPlaySession, name: "Direct Play", colour: "direct-play" },
    { session: remuxSession, name: "Direct Stream", colour: "direct-stream" },
    { session: transcodeSession, name: "Transcode", colour: "transcode" },
  ])("shows $name in its status colour with text and an icon", ({ session, name, colour }) => {
    renderBadge(session);

    const badge = screen.getByRole("button", { name: `Stream info, mode: ${name}` });
    expect(badge.textContent).toBe(name);
    expect(badge.className).toContain(`text-${colour}`);
    expect(badge.querySelector("svg")).not.toBeNull();
  });

  it("uses an icon of its own per mode, so colour is not the only signal", () => {
    const icons = [directPlaySession, remuxSession, transcodeSession].map((session) => {
      const { container } = render(
        <I18nextProvider i18n={createAppI18n(["en"])}>
          <ModeBadge
            info={streamInfo(session)}
            open={false}
            onToggle={vi.fn()}
            buttonRef={createRef<HTMLButtonElement>()}
          />
        </I18nextProvider>,
      );
      return container.querySelector("svg")?.innerHTML;
    });

    expect(new Set(icons).size).toBe(3);
  });

  it("updates when the session changes its mode", () => {
    const { rerenderWith } = renderBadge(directPlaySession);

    rerenderWith(transcodeSession);

    expect(screen.getByRole("button", { name: "Stream info, mode: Transcode" })).toBeTruthy();
    expect(screen.queryByText("Direct Play")).toBeNull();
  });

  it("describes the button with the full reason, reachable by hover and focus", () => {
    renderBadge(transcodeSession);

    const badge = screen.getByRole("button", { name: /^Stream info/ });
    const tooltip = screen.getByRole("tooltip", { hidden: true });
    expect(badge.getAttribute("aria-describedby")).toBe(tooltip.id);
    expect(tooltip.textContent).toContain("Video HEVC not supported → H.264");
    expect(tooltip.className).toContain("group-hover:block");
    expect(tooltip.className).toContain("group-focus-within:block");
  });

  it("says nothing needs converting when there is no reason", () => {
    renderBadge(directPlaySession);

    expect(screen.getByRole("tooltip", { hidden: true }).textContent).toBe(
      "Nothing is converted: the file is delivered as it is.",
    );
  });

  it("reports the open state and toggles on click", () => {
    const { onToggle } = renderBadge(remuxSession, "en", true);

    const badge = screen.getByRole("button", { name: /^Stream info/ });
    expect(badge.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(badge);

    expect(onToggle).toHaveBeenCalledOnce();
  });

  it("speaks German", () => {
    renderBadge(transcodeSession, "de");

    expect(screen.getByRole("button", { name: "Stream-Info, Modus: Transcode" })).toBeTruthy();
  });

  it("has no serious accessibility violations", async () => {
    renderBadge(transcodeSession);

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});
