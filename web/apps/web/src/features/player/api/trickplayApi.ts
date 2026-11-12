import { ApiError, requestData, toApiError, type ApiClient } from "@mivo/api-client";
import type { TrickplayLayout } from "@mivo/player-core";

const manifestPath = "/api/v1/media-files/{fileId}/trickplay";
const notFoundStatus = 404;

/** The two trickplay calls the player makes (ADR 0030 section 8). */
export interface TrickplayApi {
  /** The manifest, or `undefined` when the file has no trickplay (yet): 404. */
  readonly getManifest: (
    fileId: string,
    signal: AbortSignal,
  ) => Promise<TrickplayLayout | undefined>;
  /** Asks the server to generate the trickplay; answers 202 at once. */
  readonly requestGeneration: (fileId: string, signal: AbortSignal) => Promise<void>;
}

/** The trickplay endpoints through the generated API client. */
export function createTrickplayApi(client: ApiClient): TrickplayApi {
  return {
    async getManifest(fileId, signal) {
      try {
        return await requestData(
          client.GET(manifestPath, { params: { path: { fileId } }, signal }),
        );
      } catch (error) {
        if (error instanceof ApiError && error.status === notFoundStatus) {
          return undefined;
        }
        throw error;
      }
    },
    async requestGeneration(fileId, signal) {
      // 202 has no body, which requestData reports as an error.
      const result = await client.POST(manifestPath, { params: { path: { fileId } }, signal });
      if (!result.response.ok) {
        throw toApiError(result);
      }
    },
  };
}
