import type { components } from "./generated/schema";

type Schemas = components["schemas"];

/** Aggregated server health; the body of both 200 and 503 from `GET /api/v1/health`. */
export type HealthResponse = Schemas["HealthResponse"];

/** Result of one health check inside a {@link HealthResponse}. */
export type HealthCheck = Schemas["HealthCheck"];

/** State of the server or of one check. Later API versions may add values. */
export type HealthStatus = Schemas["HealthStatus"];

/** Product name, version and identity of the running server. */
export type SystemInfo = Schemas["SystemInfo"];

/** RFC 9457 problem details with the machine-readable `code` clients translate. */
export type Problem = Schemas["Problem"];
