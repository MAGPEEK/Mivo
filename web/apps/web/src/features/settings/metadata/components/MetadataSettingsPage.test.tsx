import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { blockingAxeViolations } from "../../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../../test/fakeApi";
import { renderApp } from "../../../../test/renderApp";
import type {
  MetadataSettings,
  MetadataSettingsUpdate,
  MetadataStatus,
} from "../api/metadataSettings";

const metadataPath = "/admin/settings/metadata";
const settingsPath = "/api/v1/settings/metadata";
const statusPath = "/api/v1/metadata/status";
const notImplementedStatus = 501;
const serverErrorStatus = 500;
const badRequestStatus = 400;
const secretKey = "0123456789abcdef-secret";

const initialSettings: MetadataSettings = {
  onlineEnabled: true,
  language: "en",
  providers: [
    { provider: "tmdb", enabled: true, keyStatus: "bundled" },
    { provider: "musicbrainz", enabled: true },
    { provider: "coverartarchive", enabled: true },
  ],
};

const knownStates: MetadataStatus = {
  queueLength: 12,
  providers: [
    { provider: "tmdb", state: "rate_limited" },
    { provider: "musicbrainz", state: "ok" },
  ],
};

// Untyped, so it can carry a state a later server version may add.
const busyStatus = {
  ...knownStates,
  providers: [...knownStates.providers, { provider: "coverartarchive", state: "maintenance" }],
};

interface ServerOptions {
  readonly settings?: () => Response;
  readonly status?: () => Response;
  readonly update?: (body: MetadataSettingsUpdate) => Response | undefined;
}

/** A fake server that applies PUT bodies to its settings and records them. */
function metadataServer(options: ServerOptions = {}) {
  let settings = initialSettings;
  const updates: MetadataSettingsUpdate[] = [];
  const routes: FakeRoute[] = [
    {
      method: "GET",
      path: settingsPath,
      respond: () => options.settings?.() ?? jsonResponse(settings),
    },
    {
      method: "GET",
      path: statusPath,
      respond: () => options.status?.() ?? jsonResponse(busyStatus),
    },
    {
      method: "PUT",
      path: settingsPath,
      respond: async ({ request }) => {
        const body = (await request.json()) as MetadataSettingsUpdate;
        updates.push(body);
        const override = options.update?.(body);
        if (override !== undefined) {
          return override;
        }
        settings = applyUpdate(settings, body);
        return jsonResponse(settings);
      },
    },
  ];
  return { server: createFakeServer(routes), updates };
}

function applyUpdate(settings: MetadataSettings, body: MetadataSettingsUpdate): MetadataSettings {
  const toggles = body.providers ?? [];
  return {
    onlineEnabled: body.onlineEnabled ?? settings.onlineEnabled,
    language: body.language ?? settings.language,
    providers: settings.providers.map((setting) => {
      const toggle = toggles.find((entry) => entry.provider === setting.provider);
      const enabled = toggle?.enabled ?? setting.enabled;
      if (setting.provider !== "tmdb" || body.tmdbApiKey === undefined) {
        return { ...setting, enabled };
      }
      return { ...setting, enabled, keyStatus: body.tmdbApiKey === "" ? "bundled" : "own" };
    }),
  };
}

async function findForm(): Promise<HTMLElement> {
  await screen.findByRole("switch", { name: "Look up metadata online" });
  return screen.getByRole("main");
}

describe("MetadataSettingsPage", () => {
  it("shows the switches, key status, language and provider status", async () => {
    const { server } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    const main = await findForm();

    expect(within(main).getByRole("heading", { level: 1, name: "Metadata" })).toBeTruthy();
    expect(screen.getByRole("switch", { name: "TMDB (movies and shows)" })).toHaveProperty(
      "checked",
      true,
    );
    expect(
      screen.getByRole("switch", { name: "MusicBrainz and Cover Art Archive (music)" }),
    ).toHaveProperty("checked", true);
    expect(screen.getByText(/Key in use: Project key/)).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Metadata language" })).toHaveProperty(
      "value",
      "en",
    );
    expect(await screen.findByText("12 items waiting for metadata")).toBeTruthy();
    const states = within(screen.getByRole("list", { name: "Provider states" }))
      .getAllByRole("listitem")
      .map((item) => item.textContent);
    expect(states).toEqual(["TMDBBusy, will retry", "MusicBrainzOK", "Cover Art ArchiveUnknown"]);
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("names the contacted hosts and what is sent", async () => {
    const { server } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    const note = await screen.findByRole("region", { name: "What goes online" });

    expect(within(note).getByText(/api\.themoviedb\.org/)).toBeTruthy();
    expect(within(note).getByText(/musicbrainz\.org/)).toBeTruthy();
    expect(within(note).getByText(/coverartarchive\.org/)).toBeTruthy();
    expect(within(note).getByText(/never file names, folder paths/)).toBeTruthy();
  });

  it("shows the TMDB credit with the bundled logo and loads nothing from outside", async () => {
    const { server } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();
    const credits = screen.getByRole("region", { name: "Data sources" });

    expect(within(credits).getByRole("img", { name: "TMDB" })).toBeTruthy();
    expect(
      within(credits).getByText(
        "This product uses the TMDB API but is not endorsed or certified by TMDB.",
      ),
    ).toBeTruthy();
    expect(within(credits).getByText(/MusicBrainz/)).toBeTruthy();
    const external = [...document.querySelectorAll("[src], [href]")].filter((element) =>
      /^(https?:)?\/\//.test(element.getAttribute("src") ?? element.getAttribute("href") ?? ""),
    );
    expect(external).toEqual([]);
  });

  it("switches online metadata off and disables the providers", async () => {
    const { server, updates } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();

    fireEvent.click(screen.getByRole("switch", { name: "Look up metadata online" }));

    await waitFor(() => {
      expect(screen.getByRole("switch", { name: "TMDB (movies and shows)" })).toHaveProperty(
        "disabled",
        true,
      );
    });
    expect(updates).toEqual([{ onlineEnabled: false }]);
    expect(screen.getByText("Switch on online metadata to choose providers.")).toBeTruthy();
  });

  it("switches MusicBrainz and the Cover Art Archive together", async () => {
    const { server, updates } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();

    const music = screen.getByRole("switch", { name: "MusicBrainz and Cover Art Archive (music)" });
    fireEvent.click(music);

    await waitFor(() => {
      expect(music).toHaveProperty("checked", false);
    });
    expect(updates).toEqual([
      {
        providers: [
          { provider: "musicbrainz", enabled: false },
          { provider: "coverartarchive", enabled: false },
        ],
      },
    ]);
  });

  it("sends an own TMDB key once and never shows it back", async () => {
    const { server, updates } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();
    const input = screen.getByLabelText("Own TMDB API key");

    expect(input).toHaveProperty("type", "password");
    fireEvent.change(input, { target: { value: `  ${secretKey}  ` } });
    fireEvent.click(screen.getByRole("button", { name: "Save key" }));

    await screen.findByText(/Key in use: Your key/);
    expect(updates).toEqual([{ tmdbApiKey: secretKey }]);
    expect(input).toHaveProperty("value", "");
    expect(document.body.innerHTML).not.toContain(secretKey);

    fireEvent.click(screen.getByRole("button", { name: "Remove own key" }));
    await screen.findByText(/Key in use: Project key/);
    expect(updates.at(-1)).toEqual({ tmdbApiKey: "" });
  });

  it("keeps a typed key when a switch is saved meanwhile", async () => {
    const { server, updates } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();
    const input = screen.getByLabelText("Own TMDB API key");

    fireEvent.change(input, { target: { value: secretKey } });
    fireEvent.click(
      screen.getByRole("switch", { name: "MusicBrainz and Cover Art Archive (music)" }),
    );

    await waitFor(() => {
      expect(updates).toHaveLength(1);
    });
    await waitFor(() => {
      expect(
        screen.getByRole("switch", { name: "MusicBrainz and Cover Art Archive (music)" }),
      ).toHaveProperty("checked", false);
    });
    expect(input).toHaveProperty("value", secretKey);
  });

  it("reports a rejected key next to the field", async () => {
    const { server } = metadataServer({
      update: (body) =>
        body.tmdbApiKey === undefined
          ? undefined
          : problemResponse(serverErrorStatus, "internal_error"),
    });
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();

    fireEvent.change(screen.getByLabelText("Own TMDB API key"), { target: { value: secretKey } });
    fireEvent.click(screen.getByRole("button", { name: "Save key" }));

    expect((await screen.findByRole("alert")).textContent).toBe("The change could not be saved.");
  });

  it("keeps Save key disabled until a key is typed", async () => {
    const { server } = metadataServer();
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();

    expect(screen.getByRole("button", { name: "Save key" })).toHaveProperty("disabled", true);
    expect(screen.queryByRole("button", { name: "Remove own key" })).toBeNull();
  });

  it("changes the language and reports a rejected one", async () => {
    const { server, updates } = metadataServer({
      update: (body) =>
        body.language === "fr" ? problemResponse(badRequestStatus, "language_invalid") : undefined,
    });
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();

    fireEvent.change(screen.getByRole("combobox", { name: "Metadata language" }), {
      target: { value: "fr" },
    });

    expect((await screen.findByRole("alert")).textContent).toBe(
      "The server does not accept this language.",
    );
    expect(updates).toEqual([{ language: "fr" }]);
  });

  it("says so when the server does not serve online metadata yet (501)", async () => {
    const { server } = metadataServer({
      settings: () => problemResponse(notImplementedStatus, "not_implemented"),
      status: () => problemResponse(notImplementedStatus, "not_implemented"),
    });
    renderApp(metadataPath, { fetch: server.fetch });

    expect(await screen.findByText(/does not support online metadata yet/)).toBeTruthy();
    expect(await screen.findByText(/No status/)).toBeTruthy();
    expect(screen.getByRole("region", { name: "What goes online" })).toBeTruthy();
    expect(screen.getByRole("img", { name: "TMDB" })).toBeTruthy();
    expect(screen.queryByRole("switch")).toBeNull();
    // Neither query retries a 501.
    expect(server.countRequests("GET", settingsPath)).toBe(1);
    expect(server.countRequests("GET", statusPath)).toBe(1);
  });

  it("offers a retry when the settings fail to load", async () => {
    let failing = true;
    const { server } = metadataServer({
      settings: () =>
        failing
          ? problemResponse(serverErrorStatus, "internal_error")
          : jsonResponse(initialSettings),
    });
    renderApp(metadataPath, { fetch: server.fetch });

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("The metadata settings could not be loaded.");
    failing = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("switch", { name: "Look up metadata online" })).toBeTruthy();
  });

  it.each([
    {
      status: 403,
      code: "forbidden",
      text: "Metadata settings are only available from the local network until accounts exist.",
    },
    { status: serverErrorStatus, code: "internal_error", text: "The change could not be saved." },
    {
      status: notImplementedStatus,
      code: "not_implemented",
      text: "This server version does not support online metadata yet.",
    },
  ])("reports a rejected change ($code)", async ({ status, code, text }) => {
    const { server } = metadataServer({ update: () => problemResponse(status, code) });
    renderApp(metadataPath, { fetch: server.fetch });
    await findForm();

    fireEvent.click(screen.getByRole("switch", { name: "Look up metadata online" }));

    expect((await screen.findByRole("alert")).textContent).toBe(text);
  });

  it("says so when the status fails to load", async () => {
    const { server } = metadataServer({
      status: () => problemResponse(serverErrorStatus, "internal_error"),
    });
    renderApp(metadataPath, { fetch: server.fetch });

    expect(await screen.findByText("The status could not be loaded.")).toBeTruthy();
  });

  it("is reachable from the libraries page", async () => {
    const { server } = metadataServer();
    renderApp("/admin/manage/libraries", { fetch: server.fetch });
    const nav = await screen.findByRole("navigation", { name: "Settings pages" });

    fireEvent.click(within(nav).getByRole("link", { name: "Metadata" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Metadata" })).toBeTruthy();
  });
});
