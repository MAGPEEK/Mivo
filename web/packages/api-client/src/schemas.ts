import type { components } from "./generated/schema";

type Schemas = components["schemas"];

// Enum types list the values this client version knows. Servers of the same major version
// may send more (ADR 0017): handle unknown values with a generic fallback, never assume the
// union is exhaustive.

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

/** Body of `GET /api/v1/libraries`. */
export type LibraryList = Schemas["LibraryList"];

/** A configured library with availability, item count and scan status. */
export type Library = Schemas["Library"];

/** What a library contains. Later API versions may add values. */
export type LibraryKind = Schemas["LibraryKind"];

/** Progress and result of the current or last scan of a library. */
export type ScanStatus = Schemas["ScanStatus"];

/** State of a library scan. Later API versions may add values. */
export type ScanState = Schemas["ScanState"];

/** File counts of a scan. */
export type ScanCounts = Schemas["ScanCounts"];

/** One page of a library's items; follow `nextCursor` until it is absent. */
export type ItemPage = Schemas["ItemPage"];

/** An item in a library listing. */
export type ItemSummary = Schemas["ItemSummary"];

/** Technical summary of an item's primary file for list views. */
export type MediaSummary = Schemas["MediaSummary"];

/** An item with its media files and their streams. */
export type ItemDetail = Schemas["ItemDetail"];

/** One media file (by name, never by path) with its probed streams. */
export type MediaFile = Schemas["MediaFile"];

/** One stream of a media file. */
export type MediaStream = Schemas["MediaStream"];

/** Kind of a stream. Later API versions may add values. */
export type MediaStreamKind = Schemas["MediaStreamKind"];

/** HDR format of a video stream. Later API versions may add values. */
export type HdrFormat = Schemas["HdrFormat"];

/** Lower-case container or codec identifier (ADR 0023). */
export type MediaIdentifier = Schemas["MediaIdentifier"];

/** What a client plays (ADR 0023); sent with every {@link PlaybackRequest}. */
export type ClientProfile = Schemas["ClientProfile"];

/** A container the client plays as a progressive file. */
export type DirectPlayProfile = Schemas["DirectPlayProfile"];

/** Codecs the client plays inside HLS with fMP4 segments. */
export type HlsProfile = Schemas["HlsProfile"];

/** How the client plays HLS. Later API versions may add values. */
export type HlsDelivery = Schemas["HlsDelivery"];

/** Extra limits of the client for one codec. */
export type CodecLimit = Schemas["CodecLimit"];

/** Body of `POST /api/v1/playback/sessions`. */
export type PlaybackRequest = Schemas["PlaybackRequest"];

/** A playback session with the server's decision and the opaque `streamUrl`. */
export type PlaybackSession = Schemas["PlaybackSession"];

/** Lifecycle state of a session. Later API versions may add values. */
export type PlaybackSessionState = Schemas["PlaybackSessionState"];

/** Direct play, remux or transcode. Later API versions may add values. */
export type PlaybackMode = Schemas["PlaybackMode"];

/** How the client loads `streamUrl`. Later API versions may add values. */
export type PlaybackDelivery = Schemas["PlaybackDelivery"];

/** What a session does with one source stream. */
export type StreamDecision = Schemas["StreamDecision"];

/** Copy or transcode. Later API versions may add values. */
export type StreamAction = Schemas["StreamAction"];

/** A reason or warning code with the part of the file it concerns. */
export type PlaybackReason = Schemas["PlaybackReason"];

/** Part of the file a reason concerns. Later API versions may add values. */
export type PlaybackReasonScope = Schemas["PlaybackReasonScope"];
