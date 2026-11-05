import type { IdentifyCandidate, ItemDetail, ItemKind } from "@mivo/api-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { I18nextProvider } from "react-i18next";
import { describe, expect, it, vi } from "vitest";

import { ApiClientProvider } from "../../../shared/api";
import { createI18n } from "../../../shared/i18n";
import { blockingAxeViolations } from "../../../test/axe";
import {
  createFakeApiClient,
  createFakeServer,
  jsonResponse,
  problemResponse,
  type FakeRoute,
  type FakeServer,
} from "../../../test/fakeApi";
import { identifyLocales, identifyNamespace } from "../locales";
import { ItemMatchMenu } from "./ItemMatchMenu";

const itemId = "item-1";
const searchPath = `/api/v1/items/${itemId}/identify/search`;
const identifyPath = `/api/v1/items/${itemId}/identify`;

const bunny: IdentifyCandidate = {
  provider: "tmdb",
  externalId: "10378",
  title: "Big Buck Bunny",
  year: 2008,
  overview: "A giant rabbit takes revenge.",
  posterUrl: "/api/v1/images/poster-1?width=320",
  confidence: 0.92,
};
const otherBunny: IdentifyCandidate = {
  provider: "tmdb",
  externalId: "99",
  title: "Big Buck Bunny 2",
  confidence: 0.4,
};

const matchedItem: ItemDetail = {
  id: itemId,
  libraryId: "library-1",
  kind: "movie",
  title: "Big Buck Bunny",
  files: [],
  matchState: "matched",
};

function candidatesRoute(candidates: readonly IdentifyCandidate[]): FakeRoute {
  return { method: "POST", path: searchPath, respond: () => jsonResponse({ candidates }) };
}

interface RenderOptions {
  readonly routes: readonly FakeRoute[];
  readonly kind?: ItemKind;
  readonly year?: string;
  readonly languages?: readonly string[];
}

function renderMenu({ routes, kind = "movie", year = "2008", languages = ["en"] }: RenderOptions) {
  const server = createFakeServer(routes);
  const onItemChanged = vi.fn();
  const i18n = createI18n(languages, { [identifyNamespace]: identifyLocales });
  render(
    <I18nextProvider i18n={i18n}>
      <ApiClientProvider client={createFakeApiClient(server.fetch)}>
        <QueryClientProvider client={new QueryClient()}>
          <ItemMatchMenu
            itemId={itemId}
            kind={kind}
            title="Big Buck Bunny"
            year={year}
            onItemChanged={onItemChanged}
          />
        </QueryClientProvider>
      </ApiClientProvider>
    </I18nextProvider>,
  );
  return { server, onItemChanged };
}

function chooseMenuEntry(name: string): void {
  fireEvent.click(screen.getByRole("button", { name: "More actions" }));
  fireEvent.click(screen.getByRole("menuitem", { name }));
}

async function openIdentify(): Promise<HTMLElement> {
  chooseMenuEntry("Correct match…");
  return screen.findByRole("dialog", { name: "Correct match" });
}

async function searchBodies(server: FakeServer): Promise<unknown[]> {
  const requests = server.requests.filter(
    (request) => new URL(request.url).pathname === searchPath,
  );
  return Promise.all(requests.map((request) => request.clone().json() as Promise<unknown>));
}

describe("ItemMatchMenu", () => {
  it.each(["season", "episode", "extra"] as const)("is hidden for a %s", (kind) => {
    renderMenu({ routes: [], kind });

    expect(screen.queryByRole("button", { name: "More actions" })).toBeNull();
  });

  it("offers correct match, remove match and refresh", () => {
    renderMenu({ routes: [] });

    fireEvent.click(screen.getByRole("button", { name: "More actions" }));

    expect(screen.getAllByRole("menuitem").map((entry) => entry.textContent)).toEqual([
      "Correct match…",
      "Remove match",
      "Refresh metadata",
    ]);
  });
});

describe("IdentifyDialog search", () => {
  it("searches at once with title and year and lists the candidates", async () => {
    const { server } = renderMenu({ routes: [candidatesRoute([bunny, otherBunny])] });

    const dialog = await openIdentify();

    expect(within(dialog).getByRole("searchbox")).toHaveProperty("value", "Big Buck Bunny");
    expect(within(dialog).getByRole("textbox", { name: "Year" })).toHaveProperty("value", "2008");
    const results = await within(dialog).findByRole("list", { name: "Results" });
    const rows = within(results).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    const [first] = rows;
    if (first === undefined) {
      throw new Error("no first row");
    }
    expect(within(first).getByRole("heading", { name: "Big Buck Bunny (2008)" })).toBeTruthy();
    expect(within(first).getByText("TMDB")).toBeTruthy();
    expect(within(first).getByText("A giant rabbit takes revenge.")).toBeTruthy();
    expect(first.querySelector("img")?.getAttribute("src")).toBe(bunny.posterUrl);
    expect(await searchBodies(server)).toEqual([{ query: "Big Buck Bunny", year: 2008 }]);
  });

  it("focuses the search field when it opens", async () => {
    renderMenu({ routes: [candidatesRoute([])] });

    const dialog = await openIdentify();

    expect(document.activeElement).toBe(within(dialog).getByRole("searchbox"));
  });

  it("sends a pasted link as providerId", async () => {
    const { server } = renderMenu({ routes: [candidatesRoute([bunny])] });
    const dialog = await openIdentify();
    await within(dialog).findByRole("list", { name: "Results" });
    const link = "https://www.themoviedb.org/movie/10378";

    fireEvent.change(within(dialog).getByRole("searchbox"), { target: { value: link } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Search" }));

    await waitFor(async () => {
      expect(await searchBodies(server)).toHaveLength(2);
    });
    expect((await searchBodies(server))[1]).toEqual({
      query: "Big Buck Bunny",
      providerId: link,
    });
  });

  it("searches again when the same search is submitted", async () => {
    const { server } = renderMenu({ routes: [candidatesRoute([bunny])] });
    const dialog = await openIdentify();
    await within(dialog).findByRole("list", { name: "Results" });

    fireEvent.click(within(dialog).getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(server.countRequests("POST", searchPath)).toBe(2);
    });
  });

  it("says when there are no results", async () => {
    renderMenu({ routes: [candidatesRoute([])] });

    const dialog = await openIdentify();

    expect(
      await within(dialog).findByText("No results. Try another title or leave the year empty."),
    ).toBeTruthy();
  });

  it("explains an offline provider (503) and retries on request", async () => {
    let answer = () => problemResponse(503, "provider_unavailable");
    const { server } = renderMenu({
      routes: [{ method: "POST", path: searchPath, respond: () => answer() }],
    });
    const dialog = await openIdentify();

    expect(await within(dialog).findByText("Provider offline, try again later.")).toBeTruthy();
    answer = () => jsonResponse({ candidates: [bunny] });
    fireEvent.click(within(dialog).getByRole("button", { name: "Try again" }));

    expect(await within(dialog).findByRole("list", { name: "Results" })).toBeTruthy();
    expect(server.countRequests("POST", searchPath)).toBe(2);
  });

  it.each([
    [409, "online_disabled", "Online metadata is switched off – Settings › Metadata."],
    [501, "not_implemented", "This server does not support online metadata yet."],
    [403, "forbidden", "Only a device on the server's local network can change metadata."],
  ])("explains %s %s without a retry", async (status, code, text) => {
    renderMenu({
      routes: [{ method: "POST", path: searchPath, respond: () => problemResponse(status, code) }],
    });

    const dialog = await openIdentify();

    await within(dialog).findByText(text);
    expect(within(dialog).queryByRole("button", { name: "Try again" })).toBeNull();
  });

  it("shows German texts", async () => {
    renderMenu({ routes: [candidatesRoute([])], languages: ["de"] });

    fireEvent.click(screen.getByRole("button", { name: "Weitere Aktionen" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Zuordnung korrigieren…" }));

    expect(await screen.findByRole("dialog", { name: "Zuordnung korrigieren" })).toBeTruthy();
  });

  it("has no serious or critical axe findings with results", async () => {
    renderMenu({ routes: [candidatesRoute([bunny, otherBunny])] });
    const dialog = await openIdentify();
    await within(dialog).findByRole("list", { name: "Results" });

    expect(await blockingAxeViolations(document.body)).toEqual([]);
  });
});

describe("IdentifyDialog apply", () => {
  it("applies a candidate, closes and reports the matched item", async () => {
    let release: () => void = () => undefined;
    const answered = new Promise<void>((resolve) => {
      release = resolve;
    });
    const { server, onItemChanged } = renderMenu({
      routes: [
        candidatesRoute([bunny, otherBunny]),
        {
          method: "POST",
          path: identifyPath,
          respond: async () => {
            await answered;
            return jsonResponse(matchedItem);
          },
        },
      ],
    });
    const dialog = await openIdentify();
    await within(dialog).findByRole("list", { name: "Results" });

    fireEvent.click(within(dialog).getByRole("button", { name: "Apply Big Buck Bunny (2008)" }));

    expect(await within(dialog).findByText("Applying the match…")).toBeTruthy();
    const otherApply = within(dialog).getByRole("button", { name: "Apply Big Buck Bunny 2" });
    expect(otherApply.getAttribute("aria-disabled")).toBe("true");
    // Closing is blocked while applying: the server would apply the match anyway.
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.getByRole("dialog")).toBe(dialog);
    release();
    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(onItemChanged).toHaveBeenCalledWith(matchedItem);
    expect(screen.getByRole("status").textContent).toContain(
      "Match applied. The metadata is being updated.",
    );
    const identifyRequest = server.requests.find(
      (request) => new URL(request.url).pathname === identifyPath,
    );
    expect(await identifyRequest?.clone().json()).toEqual({
      provider: "tmdb",
      externalId: "10378",
    });
  });

  it("keeps the dialog open and explains a failed apply", async () => {
    const { onItemChanged } = renderMenu({
      routes: [
        candidatesRoute([bunny]),
        {
          method: "POST",
          path: identifyPath,
          respond: () => problemResponse(503, "provider_unavailable"),
        },
      ],
    });
    const dialog = await openIdentify();
    await within(dialog).findByRole("list", { name: "Results" });

    fireEvent.click(within(dialog).getByRole("button", { name: "Apply Big Buck Bunny (2008)" }));

    expect((await within(dialog).findByRole("alert")).textContent).toContain(
      "Provider offline, try again later.",
    );
    expect(onItemChanged).not.toHaveBeenCalled();

    fireEvent.click(within(dialog).getByRole("button", { name: "Search" }));

    await waitFor(() => {
      expect(within(dialog).queryByText("Provider offline, try again later.")).toBeNull();
    });
  });

  it("closes with Cancel without changing anything", async () => {
    const { onItemChanged } = renderMenu({ routes: [candidatesRoute([])] });
    const dialog = await openIdentify();

    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "More actions" }));
    expect(onItemChanged).not.toHaveBeenCalled();
  });
});

describe("Remove match", () => {
  const removeRoute = (respond: () => Response): FakeRoute => ({
    method: "DELETE",
    path: `/api/v1/items/${itemId}/match`,
    respond,
  });

  it("asks first, explains that local data stays and removes on confirm", async () => {
    const unmatched: ItemDetail = { ...matchedItem, matchState: "disabled" };
    const { onItemChanged } = renderMenu({ routes: [removeRoute(() => jsonResponse(unmatched))] });

    chooseMenuEntry("Remove match");
    const dialog = await screen.findByRole("dialog", { name: "Remove match?" });
    expect(dialog.textContent).toContain("Local data stays");
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Cancel" }));

    fireEvent.click(within(dialog).getByRole("button", { name: "Remove match" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull();
    });
    expect(onItemChanged).toHaveBeenCalledWith(unmatched);
    expect(screen.getByRole("status").textContent).toContain("Match removed. Local data was kept.");
  });

  it("shows why removing failed inside the dialog", async () => {
    renderMenu({ routes: [removeRoute(() => problemResponse(501, "not_implemented"))] });

    chooseMenuEntry("Remove match");
    const dialog = await screen.findByRole("dialog", { name: "Remove match?" });
    fireEvent.click(within(dialog).getByRole("button", { name: "Remove match" }));

    expect((await within(dialog).findByRole("alert")).textContent).toContain(
      "This server does not support online metadata yet.",
    );
  });
});

describe("Refresh metadata", () => {
  const refreshRoute = (respond: () => Response): FakeRoute => ({
    method: "POST",
    path: `/api/v1/items/${itemId}/refresh`,
    respond,
  });

  it("queues a refresh (202 without body) and reloads the item", async () => {
    const { onItemChanged } = renderMenu({
      routes: [refreshRoute(() => new Response(null, { status: 202 }))],
    });

    chooseMenuEntry("Refresh metadata");

    expect(await screen.findByText("Metadata refresh queued.")).toBeTruthy();
    expect(onItemChanged).toHaveBeenCalledWith(undefined);
  });

  it("explains switched-off online metadata", async () => {
    const { onItemChanged } = renderMenu({
      routes: [refreshRoute(() => problemResponse(409, "online_disabled"))],
    });

    chooseMenuEntry("Refresh metadata");

    expect(
      await screen.findByText(
        "The metadata could not be refreshed: Online metadata is switched off – Settings › Metadata.",
      ),
    ).toBeTruthy();
    expect(onItemChanged).not.toHaveBeenCalled();
  });

  it("explains an unreachable server", async () => {
    renderMenu({
      routes: [
        refreshRoute(() => {
          throw new TypeError("Failed to fetch");
        }),
      ],
    });

    chooseMenuEntry("Refresh metadata");

    expect(
      await screen.findByText("The metadata could not be refreshed: The server cannot be reached."),
    ).toBeTruthy();
  });
});
