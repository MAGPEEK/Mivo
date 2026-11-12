// A local stand-in for the TMDB API and image host (task M6-17). The E2E server is pointed at it
// by the test-only `MIVO_E2E_TMDB_URL` variable (cmd/mivo/hooks_e2e.go), so the suite never
// reaches the internet. It records every request, which lets a spec prove that the server
// contacted the provider, or that it did not.
import { readFile } from "node:fs/promises";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import { join } from "node:path";

import { repositoryRoot } from "./commands";
import { loopbackHost } from "./server";

/** Variable that tells the E2E server where its fake TMDB listens. */
export const fakeTmdbUrlVariable = "MIVO_E2E_TMDB_URL";

const posterFile = join(repositoryRoot, "testdata", "providers", "images", "poster.jpg");
const notFoundStatus = 404;
const okStatus = 200;
const searchMoviePath = "/3/search/movie";
const moviePathPattern = /^\/3\/movie\/(\d+)$/;
const imagePathPrefix = "/t/p/";
const emptyPage = { page: 1, results: [], movie_results: [], tv_results: [] };

/** Movie title that the fake finds two equally good entries for, so it stays "Unmatched". */
export const ambiguousTitle = "Kill Bill";
/** The entry the correct-match flow picks for {@link ambiguousTitle}. */
export const ambiguousPick = { id: 24, title: "Kill Bill: Vol. 1", year: 2003 };
const ambiguousOther = { id: 393, title: "Kill Bill: Vol. 2", year: 2004 };

/** Overview the fake gives every movie, so a spec can tell provider data from local data. */
export const fakeOverview = "Overview from the fake TMDB.";

interface FakeMovie {
  readonly id: number;
  readonly title: string;
  readonly year: number;
}

/** One request the fake received. */
export interface FakeRequest {
  readonly method: string;
  readonly path: string;
  /** The `query` parameter of a search, empty for every other request. */
  readonly query: string;
}

/** A running fake TMDB that can be stopped and started again on the same port. */
export interface FakeTmdb {
  /** Origin such as `http://127.0.0.1:53124`; stable across {@link FakeTmdb.stop} and start. */
  readonly origin: string;
  /** Every request since the last {@link FakeTmdb.clearRequests}. */
  readonly requests: () => readonly FakeRequest[];
  readonly clearRequests: () => void;
  /** Refuses connections until {@link FakeTmdb.start} is called again. */
  readonly stop: () => Promise<void>;
  readonly start: () => Promise<void>;
}

/** Starts the fake on a free loopback port. */
export async function startFakeTmdb(): Promise<FakeTmdb> {
  const poster = await readFile(posterFile);
  const state = new FakeState(poster);
  const server = createServer((request, response) => {
    state.handle(request, response);
  });
  await listen(server, 0);
  const address = server.address();
  if (address === null || typeof address === "string") {
    throw new Error("The fake TMDB has no TCP address.");
  }
  const port = address.port;
  const holder = { server };
  return {
    origin: `http://${loopbackHost}:${String(port)}`,
    requests: () => state.requests,
    clearRequests: () => {
      state.requests = [];
    },
    stop: () => close(holder.server),
    start: async () => {
      holder.server = createServer((request, response) => {
        state.handle(request, response);
      });
      await listen(holder.server, port);
    },
  };
}

function listen(server: Server, port: number): Promise<void> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, loopbackHost, () => {
      server.off("error", reject);
      resolve();
    });
  });
}

function close(server: Server): Promise<void> {
  return new Promise((resolve) => {
    server.close(() => {
      resolve();
    });
    // Keep-alive connections of the server would hold the port open for a long time.
    server.closeAllConnections();
  });
}

/** The answers: a search finds the queried title as its only entry, except the ambiguous one. */
class FakeState {
  requests: FakeRequest[] = [];
  private readonly movies = new Map<number, FakeMovie>();

  constructor(private readonly poster: Buffer) {
    for (const movie of [ambiguousPick, ambiguousOther]) {
      this.movies.set(movie.id, movie);
    }
  }

  handle(request: IncomingMessage, response: ServerResponse): void {
    const url = new URL(request.url ?? "/", "http://fake.invalid");
    this.requests.push({
      method: request.method ?? "GET",
      path: url.pathname,
      query: url.searchParams.get("query") ?? "",
    });
    if (url.pathname === searchMoviePath) {
      this.searchMovie(url, response);
    } else if (url.pathname.startsWith("/3/search/") || url.pathname.startsWith("/3/find/")) {
      sendJson(response, emptyPage);
    } else if (moviePathPattern.test(url.pathname)) {
      this.movie(url.pathname, response);
    } else if (url.pathname.startsWith(imagePathPrefix)) {
      response.writeHead(okStatus, { "Content-Type": "image/jpeg" }).end(this.poster);
    } else {
      response.writeHead(notFoundStatus).end();
    }
  }

  private searchMovie(url: URL, response: ServerResponse): void {
    const title = url.searchParams.get("query") ?? "";
    const year = Number(url.searchParams.get("year") ?? "0");
    if (title === ambiguousTitle) {
      sendJson(response, { page: 1, results: [ambiguousPick, ambiguousOther].map(movieJson) });
      return;
    }
    const movie = { id: idFor(title), title, year };
    this.movies.set(movie.id, movie);
    sendJson(response, { page: 1, results: [movieJson(movie)] });
  }

  private movie(path: string, response: ServerResponse): void {
    const id = Number(moviePathPattern.exec(path)?.[1]);
    // An ID the fake never listed comes from an NFO file of the corpus: it exists at the provider.
    sendJson(
      response,
      movieJson(this.movies.get(id) ?? { id, title: `Movie ${String(id)}`, year: 0 }),
    );
  }
}

// A stable ID per title, so the same query finds the same entry again after a restart.
function idFor(title: string): number {
  let hash = 0;
  for (const character of title) {
    hash = (hash * 31 + (character.codePointAt(0) ?? 0)) % 1_000_000;
  }
  return hash + 1000;
}

function movieJson(movie: FakeMovie) {
  return {
    id: movie.id,
    title: movie.title,
    original_title: movie.title,
    popularity: 10,
    overview: fakeOverview,
    poster_path: `/poster${String(movie.id)}.jpg`,
    ...(movie.year > 0 ? { release_date: `${String(movie.year).padStart(4, "0")}-01-01` } : {}),
  };
}

function sendJson(response: ServerResponse, body: unknown): void {
  response.writeHead(okStatus, { "Content-Type": "application/json" }).end(JSON.stringify(body));
}
