import { expect, test as base, type Locator, type Page } from "@playwright/test";
import { join } from "node:path";

import { blockingAxeViolations } from "./support/axe";
import {
  expectedPhotos,
  libraryCorpusDirectory,
  missingPhotoCorpusReason,
  photoCorpusExists,
  type ExpectedPhoto,
} from "./support/libraryCorpus";
import { waitForStartupScans } from "./support/scans";
import { buildTimeoutMs, startE2EServer, startupTimeoutMs, type E2EServer } from "./support/server";
import { openApp } from "./support/shell";

// Photos in a real browser (task M3-15i): a photos library from the corpus lists its albums,
// an album shows its sub-albums and its photos as thumbnails the browser decodes, a photo the
// server cannot render shows a placeholder, and a photo opens large with its date and camera.
// The suite owns its server, so the other specs' corpus stays untouched.

const photoLibraryName = "Pictures";
const scanTimeoutMs = 60_000;
const fixtureTimeoutMs = buildTimeoutMs + startupTimeoutMs + scanTimeoutMs;
const suiteTestTimeoutMs = 60_000;
const holidaysAlbum = "Holidays 2024";
const lakeShoreAlbum = "Lake Shore";
const rawOnlyAlbum = "Raw Only";

interface Fixtures {
  readonly baseURL: string;
}

interface WorkerFixtures {
  readonly photoServer: E2EServer;
}

const test = base.extend<Fixtures, WorkerFixtures>({
  photoServer: [
    // Playwright needs the destructuring pattern to see that the fixture has no dependencies.
    // eslint-disable-next-line no-empty-pattern
    async ({}, provide, workerInfo) => {
      const server = await startE2EServer({
        logFile: join(workerInfo.project.outputDir, "server-photos.log"),
        libraries: [
          {
            name: photoLibraryName,
            kind: "photos",
            copyFrom: join(libraryCorpusDirectory, expectedPhotos().path),
          },
        ],
      });
      try {
        await waitForStartupScans(server.baseUrl, scanTimeoutMs);
        await provide(server);
      } finally {
        await server.stop();
      }
    },
    { scope: "worker", timeout: fixtureTimeoutMs },
  ],
  baseURL: async ({ photoServer }, provide) => {
    await provide(photoServer.baseUrl);
  },
});

if (!photoCorpusExists()) {
  // A skipped suite must not let a CI run pass unnoticed.
  if (process.env["CI"] !== undefined) {
    throw new Error(missingPhotoCorpusReason);
  }
  test.skip(true, missingPhotoCorpusReason);
}
test.describe.configure({ timeout: suiteTestTimeoutMs });

// The top-level albums of the corpus: the first folder of every photo's album.
function topLevelAlbums(): string[] {
  const names = expectedPhotos().photos.map((photo) => photo.album.split("/")[0] ?? "");
  return [...new Set(names)].filter((name) => name !== "");
}

function corpusPhoto(file: string): ExpectedPhoto {
  const photo = expectedPhotos().photos.find((candidate) => candidate.file === file);
  if (photo === undefined) {
    throw new Error(`The photo corpus has no ${file}.`);
  }
  return photo;
}

// The file name without extension: the start of a photo's title and of its tile's name.
function baseName(file: string): string {
  const name = file.split("/").at(-1) ?? file;
  return name.replace(/\.[^.]+$/, "");
}

// Matches a title that starts with the file's base name; the scan's title cleanup turns
// underscores into spaces ("DSC_0300.nef" is listed as "DSC 0300").
function startsWith(prefix: string): RegExp {
  const escaped = prefix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replaceAll("_", "[_ ]");
  return new RegExp(`^${escaped}`);
}

async function openPhotoLibrary(page: Page): Promise<Locator> {
  const { sidebar } = await openApp(page);
  await sidebar.getByRole("link", { name: photoLibraryName, exact: true }).click();
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { level: 1, name: photoLibraryName })).toBeVisible();
  return main.getByRole("list", { name: "Photo albums" });
}

async function openAlbum(page: Page, name: string): Promise<Locator> {
  const albums = await openPhotoLibrary(page);
  await albums.getByRole("link", { name, exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name })).toBeVisible();
  return page.getByRole("main").getByRole("list", { name: "Album contents" });
}

// Thumbnails load lazily: scrolled into view, the browser must fetch and decode the image.
async function expectDecoded(image: Locator): Promise<void> {
  await image.scrollIntoViewIfNeeded();
  await expect
    .poll(() =>
      image.evaluate((element: HTMLImageElement) => element.complete && element.naturalWidth > 0),
    )
    .toBe(true);
}

test("lists the top-level albums of the corpus as square covers", async ({ page }) => {
  const albums = await openPhotoLibrary(page);
  const expected = topLevelAlbums();

  await expect(albums.getByRole("listitem")).toHaveCount(expected.length);
  for (const name of expected) {
    await expect(albums.getByRole("link", { name, exact: true })).toBeVisible();
  }
  const cover = albums
    .getByRole("listitem")
    .filter({ has: page.getByRole("link", { name: holidaysAlbum, exact: true }) })
    .locator("img");
  await expectDecoded(cover);
  expect(await blockingAxeViolations(page)).toEqual([]);
});

test("opens an album, decodes its thumbnails and opens one photo large", async ({ page }) => {
  const photo = corpusPhoto(`${holidaysAlbum}/IMG_0101.jpg`);
  const contents = await openAlbum(page, holidaysAlbum);

  // Sub-albums come first, then the photos.
  const links = contents.getByRole("link");
  await expect(links.first()).toHaveAccessibleName(lakeShoreAlbum);
  const tile = contents.getByRole("link", { name: startsWith(baseName(photo.file)) });
  await expect(tile).toBeVisible();
  const thumbnails = contents.locator("img");
  expect(await thumbnails.count()).toBeGreaterThan(0);
  for (const thumbnail of await thumbnails.all()) {
    await expectDecoded(thumbnail);
  }
  expect(await blockingAxeViolations(page)).toEqual([]);

  await tile.click();
  const heading = page.getByRole("heading", { level: 1, name: startsWith(baseName(photo.file)) });
  await expect(heading).toBeVisible();
  const large = page.getByRole("main").getByRole("img", { name: startsWith(baseName(photo.file)) });
  await expectDecoded(large);
  const details = page.getByRole("region", { name: "Photo details" });
  // EXIF times are the camera's wall clock and are shown as such (UTC, no zone shift).
  await expect(details.getByText("July 14, 2024 at 9:30 AM", { exact: true })).toBeVisible();
  await expect(
    details.getByText(`${photo.exif?.make ?? ""} ${photo.exif?.model ?? ""}`, { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("link", { name: holidaysAlbum, exact: true })).toBeVisible();
  expect(await blockingAxeViolations(page)).toEqual([]);
});

test("shows a placeholder for a photo the server cannot render", async ({ page }) => {
  const unreadable = expectedPhotos().photos.find(
    (photo) => photo.album === rawOnlyAlbum && photo.thumbnailState === "unsupported",
  );
  const readable = expectedPhotos().photos.find(
    (photo) => photo.album === rawOnlyAlbum && photo.renders,
  );
  if (unreadable === undefined || readable === undefined) {
    throw new Error(`The album ${rawOnlyAlbum} lacks a readable or an unreadable RAW.`);
  }
  const contents = await openAlbum(page, rawOnlyAlbum);

  const placeholder = contents.getByRole("link", { name: startsWith(baseName(unreadable.file)) });
  await expect(placeholder).toHaveAccessibleName(/cannot be shown$/);
  await expect(placeholder.getByText("Cannot be shown", { exact: true })).toBeVisible();
  await expect(placeholder.locator("img")).toHaveCount(0);
  // The readable RAW beside it shows through its embedded preview.
  const preview = contents.getByRole("link", { name: startsWith(baseName(readable.file)) });
  await expectDecoded(preview.locator("img"));
});
