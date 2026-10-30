import { describe, expect, it } from "vitest";

import { productId, productName, repositoryUrl } from "./index";

describe("brand", () => {
  it("has a non-blank product name", () => {
    expect(productName.trim()).not.toBe("");
  });

  it("has a lowercase product id that is safe for storage keys and paths", () => {
    expect(productId).toMatch(/^[a-z][a-z0-9-]*$/);
  });

  // A regular expression instead of `URL`, because the package has no DOM or Node types.
  it("links to the repository over HTTPS", () => {
    expect(repositoryUrl).toMatch(/^https:\/\/[a-z0-9.-]+\.[a-z]+\/\S+$/i);
  });
});
