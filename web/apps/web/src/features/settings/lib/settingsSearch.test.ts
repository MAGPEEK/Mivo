import { describe, expect, it } from "vitest";

import { validateSettingsSearch } from "./settingsSearch";

describe("validateSettingsSearch", () => {
  it.each([true, 1, "1", "true"])("reads add=%s as the add request", (add) => {
    expect(validateSettingsSearch({ add })).toEqual({ add: true });
  });

  it.each([undefined, 0, "0", "yes", false, null])("drops add=%s", (add) => {
    expect(validateSettingsSearch({ add })).toEqual({});
  });

  it("reads edit as a library id and lets add win", () => {
    expect(validateSettingsSearch({ edit: "abc" })).toEqual({ edit: "abc" });
    expect(validateSettingsSearch({ edit: "abc", add: "1" })).toEqual({ add: true });
  });

  it.each([undefined, "", 5, null])("drops edit=%s", (edit) => {
    expect(validateSettingsSearch({ edit })).toEqual({});
  });
});
