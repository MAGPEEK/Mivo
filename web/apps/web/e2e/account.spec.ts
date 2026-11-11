import { expect, test } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";

// The own account page (task M7-33) against the real server: until the server manages
// accounts it answers 501 and the page says so instead of failing; afterwards the signed-in
// owner sees the profiles of the own account.

const statusNotImplemented = 501;

test("the own account page shows the profiles or a clear not-available state", async ({ page }) => {
  const me = await page.request.get("/api/v1/me");
  await page.goto("/settings/account");

  await expect(page.getByRole("heading", { level: 1, name: "Your profiles" })).toBeVisible();
  if (me.status() === statusNotImplemented) {
    await expect(
      page.getByRole("heading", { name: "Profiles are not available yet" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Add profile" })).toHaveCount(0);
  } else {
    await expect(page.getByRole("list", { name: /^Profiles of / })).toBeVisible();
    await expect(page.getByRole("button", { name: "Add profile" })).toBeVisible();
  }
  expect(await blockingAxeViolations(page)).toEqual([]);
});
