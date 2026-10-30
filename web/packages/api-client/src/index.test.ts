import { describe, expect, it } from "vitest";

import * as apiClient from "./index";

describe("package entry", () => {
  it("exposes only the runtime API, not generated internals", () => {
    expect(Object.keys(apiClient).sort()).toEqual([
      "ApiError",
      "clientErrorCodes",
      "createApiClient",
      "noResponseStatus",
      "requestData",
      "toApiError",
      "toApiErrorFromException",
    ]);
  });
});
