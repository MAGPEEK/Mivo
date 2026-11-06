import { expect, test } from "@playwright/test";

import { blockingAxeViolations } from "./support/axe";

// Admin › Users (task M7-32) against the real server: until the server manages accounts it
// answers 501 and the page says so instead of failing; afterwards the signed-in admin lists
// at least the own account.

const statusNotImplemented = 501;

test("Admin › Users shows the accounts or a clear not-available state", async ({ page }) => {
  const users = await page.request.get("/api/v1/users");
  await page.goto("/admin/users");

  await expect(page.getByRole("heading", { level: 1, name: "Users" })).toBeVisible();
  if (users.status() === statusNotImplemented) {
    await expect(
      page.getByRole("heading", { name: "User management is not available yet" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Add user" })).toHaveCount(0);
  } else {
    await expect(page.getByRole("table", { name: "Accounts and profiles" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Add user" })).toBeVisible();
  }
  await expect(
    page.getByRole("navigation", { name: "Settings pages" }).getByRole("link", { name: "Users" }),
  ).toHaveAttribute("aria-current", "page");
  expect(await blockingAxeViolations(page)).toEqual([]);
});
