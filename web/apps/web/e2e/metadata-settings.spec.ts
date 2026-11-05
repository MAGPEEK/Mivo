import { expect, test } from "@playwright/test";

// M6-16: the TMDB logo is bundled; Settings › Metadata and About load nothing from outside
// the app origin, whether or not the server serves online metadata yet.
for (const path of ["/admin/settings/metadata", "/admin/settings/about"]) {
  test(`${path} credits TMDB with the bundled logo and stays on the app origin`, async ({
    page,
    baseURL,
  }) => {
    const origin = new URL(baseURL ?? "").origin;
    const foreignRequests: string[] = [];
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (url.protocol !== "data:" && url.origin !== origin) {
        foreignRequests.push(request.url());
      }
    });

    await page.goto(path);
    const logo = page.getByRole("img", { name: "TMDB" });
    await expect(logo).toBeVisible();
    await expect(
      page.getByText("This product uses the TMDB API but is not endorsed or certified by TMDB."),
    ).toBeVisible();
    // The logo really loaded from the app, not just an alt text in a broken image.
    await expect
      .poll(() => logo.evaluate((image: HTMLImageElement) => image.naturalWidth))
      .toBeGreaterThan(0);
    const externalAttributes = await page
      .locator("[src], [href]")
      .evaluateAll((elements) =>
        elements
          .map((element) => element.getAttribute("src") ?? element.getAttribute("href") ?? "")
          .filter((value) => /^(https?:)?\/\//.test(value)),
      );

    expect(externalAttributes).toEqual([]);
    expect(foreignRequests).toEqual([]);
  });
}
