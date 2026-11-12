import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { blockingAxeViolations } from "../../../../test/axe";
import {
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
} from "../../../../test/fakeApi";
import { renderApp } from "../../../../test/renderApp";
import {
  transcoderPollMilliseconds,
  type SessionSummary,
  type TranscoderDevice,
  type TranscoderStatus,
} from "../api/transcoderApi";

const transcoderPath = "/admin/settings/transcoder";
const statusPath = "/api/v1/system/transcoder";
const testPath = "/api/v1/system/transcoder/test";
const sessionsPath = "/api/v1/playback/sessions";
const forbiddenStatus = 403;
const conflictStatus = 409;
const serverErrorStatus = 500;
const acceptedStatus = 202;
const waitForPoll = { timeout: 4000 };

const nvencDevice: TranscoderDevice = {
  api: "nvenc",
  encoder: "h264_nvenc",
  codec: "h264",
  usable: true,
  degraded: false,
};

const testedStatus: TranscoderStatus = {
  hwaccel: "auto",
  throttleSupported: true,
  lastTestedAt: "2026-11-11T19:30:00Z",
  devices: [
    nvencDevice,
    {
      api: "vaapi",
      encoder: "h264_vaapi",
      codec: "h264",
      usable: false,
      degraded: false,
      message: "The test encode timed out.",
    },
    {
      api: "qsv",
      encoder: "h264_qsv",
      codec: "h264",
      usable: true,
      degraded: true,
      message: "<b>No device</b>",
    },
  ],
};

const transcodeSession: SessionSummary = {
  id: "5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a",
  itemId: "8a9b0c1d-2e3f-4a5b-8c6d-7e8f9a0b1c2d",
  title: "Big Buck Bunny",
  state: "ready",
  mode: "transcode",
  videoEncoder: "libx264",
  hardware: false,
  outputBitRate: 8_000_000,
  transcodeSpeed: 2.44,
  clientName: "chromium-desktop",
  startedAt: "2026-11-11T19:30:00Z",
};

const remuxSession: SessionSummary = {
  id: "0a0b0c0d-1e1f-4a2b-8c3d-4e5f6a7b8c9d",
  itemId: transcodeSession.itemId,
  title: "Sintel",
  state: "ready",
  mode: "remux",
  hardware: false,
  outputBitRate: 640_000,
  clientName: "chromium-desktop",
  startedAt: "2026-11-11T19:30:00Z",
};

interface ServerOptions {
  readonly status?: () => Response;
  readonly sessions?: () => Response;
  readonly test?: () => Response | undefined;
}

function transcoderServer(options: ServerOptions = {}) {
  const stopped: string[] = [];
  let sessions = [transcodeSession, remuxSession];
  const routes: FakeRoute[] = [
    {
      method: "GET",
      path: statusPath,
      respond: () => options.status?.() ?? jsonResponse(testedStatus),
    },
    {
      method: "GET",
      path: sessionsPath,
      respond: () => options.sessions?.() ?? jsonResponse({ sessions }),
    },
    {
      method: "POST",
      path: testPath,
      respond: () => options.test?.() ?? jsonResponse(testedStatus, acceptedStatus),
    },
    {
      method: "DELETE",
      path: `${sessionsPath}/:sessionId`,
      respond: ({ params }) => {
        stopped.push(params["sessionId"] ?? "");
        sessions = sessions.filter((session) => session.id !== params["sessionId"]);
        return new Response(null, { status: 204 });
      },
    },
  ];
  return { server: createFakeServer(routes), stopped };
}

async function findPage(): Promise<HTMLElement> {
  await screen.findByRole("button", { name: "Test hardware acceleration" });
  return screen.getByRole("main");
}

describe("TranscoderSettingsPage", () => {
  it("shows the setting, per-encoder results, throttling and the sessions", async () => {
    const { server } = transcoderServer();
    renderApp(transcoderPath, { fetch: server.fetch });
    const main = await findPage();

    expect(within(main).getByRole("heading", { level: 1, name: "Transcoder" })).toBeTruthy();
    expect(screen.getByText("Automatic")).toBeTruthy();
    expect(screen.getByText("Supported")).toBeTruthy();
    const devices = within(screen.getByRole("list", { name: "Tested encoders" }))
      .getAllByRole("listitem")
      .map((item) => item.textContent);
    expect(devices).toEqual([
      "NVENC h264_nvencUsable",
      "VAAPI h264_vaapiNot availableThe test encode timed out.",
      "QSV h264_qsvDegraded<b>No device</b>",
    ]);
    expect(screen.queryByText(/software transcoding is used/)).toBeNull();

    const rows = await screen.findAllByRole("row");
    expect(rows.map((row) => row.textContent)).toEqual([
      "TitleClientModeEncoderBit rateSpeedActions",
      "Big Buck Bunnychromium-desktopTranscodelibx264 (software)8 Mbit/s2.4×Stop",
      "Sintelchromium-desktopDirect streamCopied640 kbit/s–Stop",
    ]);
    expect(await blockingAxeViolations(main)).toEqual([]);
  });

  it("renders a device message as text, never as markup", async () => {
    const { server } = transcoderServer();
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    expect(screen.getByText("<b>No device</b>")).toBeTruthy();
    expect(document.querySelector("b")).toBeNull();
  });

  it("says software transcoding is used when no encoder was found", async () => {
    const { server } = transcoderServer({
      status: () => jsonResponse({ hwaccel: "off", devices: [], throttleSupported: false }),
    });
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    expect(screen.getByText("None found – software transcoding is used.")).toBeTruthy();
    expect(screen.getByText("Off (software only)")).toBeTruthy();
    expect(screen.getByText("Not supported")).toBeTruthy();
    expect(screen.getByText("Not tested yet")).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Tested encoders" })).toBeNull();
  });

  it("says so when every encoder is unusable", async () => {
    const { server } = transcoderServer({
      status: () =>
        jsonResponse({
          ...testedStatus,
          devices: [{ ...testedStatus.devices[1] }],
        }),
    });
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    expect(screen.getByText("None of them is usable – software transcoding is used.")).toBeTruthy();
  });

  it("disables the test button while a run is active and shows the new result", async () => {
    let reads = 0;
    const finished: TranscoderStatus = {
      ...testedStatus,
      lastTestedAt: "2026-11-12T08:00:00Z",
      devices: [nvencDevice],
    };
    const { server } = transcoderServer({
      status: () => {
        reads += 1;
        // Read 1 is the page load, read 2 the first poll after the click, read 3 the finished run.
        return jsonResponse(reads > 2 ? finished : testedStatus);
      },
    });
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    fireEvent.click(screen.getByRole("button", { name: "Test hardware acceleration" }));

    const running = await screen.findByRole("button", { name: "Testing…" });
    expect(running).toHaveProperty("disabled", true);
    expect(screen.getByText("The hardware test is running.")).toBeTruthy();
    expect(
      await screen.findByText(
        "The hardware test finished. The results above are up to date.",
        {},
        waitForPoll,
      ),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Test hardware acceleration" })).toHaveProperty(
      "disabled",
      false,
    );
    expect(
      within(screen.getByRole("list", { name: "Tested encoders" })).getAllByRole("listitem"),
    ).toHaveLength(1);
    expect(server.countRequests("POST", testPath)).toBe(1);
  });

  it("waits for a run that is active already (409) instead of failing", async () => {
    const { server } = transcoderServer({
      test: () => problemResponse(conflictStatus, "conflict"),
    });
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    fireEvent.click(screen.getByRole("button", { name: "Test hardware acceleration" }));

    expect(await screen.findByRole("button", { name: "Testing…" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(screen.queryByText("The hardware test could not be started.")).toBeNull();
  });

  it("reports a test that could not be started", async () => {
    const { server } = transcoderServer({
      test: () => problemResponse(serverErrorStatus, "internal_error"),
    });
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    fireEvent.click(screen.getByRole("button", { name: "Test hardware acceleration" }));

    expect((await screen.findByRole("alert")).textContent).toBe(
      "The hardware test could not be started.",
    );
    expect(screen.getByRole("button", { name: "Test hardware acceleration" })).toHaveProperty(
      "disabled",
      false,
    );
  });

  it("stops a session and refreshes the list", async () => {
    const { server, stopped } = transcoderServer();
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    fireEvent.click(await screen.findByRole("button", { name: "Stop Big Buck Bunny" }));

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Stop Big Buck Bunny" })).toBeNull();
    });
    expect(stopped).toEqual([transcodeSession.id]);
    expect(screen.getByRole("button", { name: "Stop Sintel" })).toBeTruthy();
  });

  it("says no stream runs when the list is empty", async () => {
    const { server } = transcoderServer({ sessions: () => jsonResponse({ sessions: [] }) });
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    expect(await screen.findByText("No stream is running.")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("flags a slow and a paused transcode", async () => {
    const { server } = transcoderServer({
      sessions: () =>
        jsonResponse({
          sessions: [{ ...transcodeSession, transcodeSpeed: 0.7, paused: true }],
        }),
    });
    renderApp(transcoderPath, { fetch: server.fetch });
    await findPage();

    expect(await screen.findByText("Slower than playback")).toBeTruthy();
    expect(screen.getByText("Paused")).toBeTruthy();
  });

  it("shows the management-guard hint once on a 403", async () => {
    const forbidden = () => problemResponse(forbiddenStatus, "forbidden");
    const { server } = transcoderServer({ status: forbidden, sessions: forbidden });
    renderApp(transcoderPath, { fetch: server.fetch });

    const hint = await screen.findByRole("alert");
    expect(hint.textContent).toBe(
      "The transcoder can only be managed by an admin on the server's local network.",
    );
    await waitFor(() => {
      expect(screen.queryByRole("heading", { name: "Active sessions" })).toBeNull();
    });
    expect(screen.queryByRole("button", { name: "Test hardware acceleration" })).toBeNull();
    // A refused caller is not asked again.
    expect(server.countRequests("GET", statusPath)).toBe(1);
  });

  it("offers a retry when the status fails to load", async () => {
    let failing = true;
    const { server } = transcoderServer({
      status: () =>
        failing ? problemResponse(serverErrorStatus, "internal_error") : jsonResponse(testedStatus),
    });
    renderApp(transcoderPath, { fetch: server.fetch });

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("The transcoder status could not be loaded.");
    failing = false;
    fireEvent.click(within(alert).getByRole("button", { name: "Try again" }));

    expect(await screen.findByRole("button", { name: "Test hardware acceleration" })).toBeTruthy();
  });

  it("asks for the sessions and the status again every 5 seconds", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const { server } = transcoderServer();
      renderApp(transcoderPath, { fetch: server.fetch });
      await findPage();
      await screen.findByText("Big Buck Bunny");
      expect(transcoderPollMilliseconds).toBe(5000);
      expect(server.countRequests("GET", sessionsPath)).toBe(1);

      await vi.advanceTimersByTimeAsync(transcoderPollMilliseconds);

      await waitFor(() => {
        expect(server.countRequests("GET", sessionsPath)).toBe(2);
      });
      expect(server.countRequests("GET", statusPath)).toBe(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it("is reachable from the settings navigation", async () => {
    const { server } = transcoderServer();
    renderApp("/admin/manage/libraries", { fetch: server.fetch });
    const nav = await screen.findByRole("navigation", { name: "Settings pages" });

    fireEvent.click(await within(nav).findByRole("link", { name: "Transcoder" }));

    expect(await screen.findByRole("heading", { level: 1, name: "Transcoder" })).toBeTruthy();
  });

  it("shows German strings", async () => {
    const { server } = transcoderServer();
    renderApp(transcoderPath, { fetch: server.fetch, languages: ["de"] });

    expect(
      await screen.findByRole("button", { name: "Hardwarebeschleunigung testen" }),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Aktive Sitzungen" })).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Big Buck Bunny beenden" })).toBeTruthy();
  });
});
