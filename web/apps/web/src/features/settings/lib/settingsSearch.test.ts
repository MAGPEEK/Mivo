import { describe, expect, it } from "vitest";

import { validateSettingsSearch } from "./settingsSearch";

describe("validateSettingsSearch", () => {
  it.each([true, 1, "1", "true"])("reads add=%s as the add request", (add) => {
    expect(validateSettingsSearch({ add })).toEqual({ add: true });
  });

  it.each([undefined, 0, "0", "yes", false, null])("drops add=%s", (add) => {
    expect(validateSettingsSearch({ add })).toEqual({});
  });
});
