import type { components, operations } from "./generated/schema";

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

/** Per-library scan and metadata switches. */
export type LibraryOptions = Schemas["LibraryOptions"];

/** Names one folder of a library: `folderId` (keep) or `rootId` with `path` (new). */
export type FolderRef = Schemas["FolderRef"];

/** Request body of `createLibrary`. */
export type LibraryCreate = Schemas["LibraryCreate"];

/** Request body of `updateLibrary`. */
export type LibraryUpdate = Schemas["LibraryUpdate"];

/** A server folder the folder browser may start from. */
export type FolderRoot = Schemas["FolderRoot"];

/** Body of `GET /api/v1/folder-roots`: roots plus install facts. */
export type FolderRootList = Schemas["FolderRootList"];

/** How the server is installed; later API versions may add values. */
export type InstallKind = Schemas["InstallKind"];

/** The sub-folders of one folder of a root. */
export type FolderListing = Schemas["FolderListing"];

/** One sub-folder of a listing. */
export type FolderEntry = Schemas["FolderEntry"];

/** Progress and result of the current or last scan of a library. */
export type ScanStatus = Schemas["ScanStatus"];

/** State of a library scan. Later API versions may add values. */
export type ScanState = Schemas["ScanState"];

/** File counts of a scan. */
export type ScanCounts = Schemas["ScanCounts"];

/** One page of a library's items; follow `nextCursor` until it is absent. */
export type ItemPage = Schemas["ItemPage"];

/** One page of the children of a library folder (folder view). */
export type LibraryFolderListing = Schemas["LibraryFolderListing"];

/** One child of a library folder: a sub-folder or a catalog item. */
export type LibraryFolderEntry = Schemas["LibraryFolderEntry"];

/** An item in a library listing. */
export type ItemSummary = Schemas["ItemSummary"];

/** Technical summary of an item's primary file for list views. */
export type MediaSummary = Schemas["MediaSummary"];

/** An item with its metadata, artwork, hierarchy, extras, media files and their streams. */
export type ItemDetail = Schemas["ItemDetail"];

/** Capture data of a photo (EXIF) for its info panel; present for photos only. */
export type ItemPhoto = Schemas["ItemPhoto"];

/** Where a photo's capture time comes from. Later API versions may add values. */
export type PhotoTimeSource = Schemas["PhotoTimeSource"];

/** Whether the server can render a photo. Later API versions may add values. */
export type ThumbnailState = Schemas["ThumbnailState"];

/** Kind of an item (movie, show, season, episode, extra). Later API versions may add values. */
export type ItemKind = Schemas["ItemKind"];

/** What an extra contains. Later API versions may add values. */
export type ExtraType = Schemas["ExtraType"];

/** An item above another one in the hierarchy, root first in `ItemDetail.ancestors`. */
export type ItemAncestor = Schemas["ItemAncestor"];

/** A person credited on an item. */
export type ItemPerson = Schemas["ItemPerson"];

/** Role of a person in an item. Later API versions may add values. */
export type PersonRole = Schemas["PersonRole"];

/** IDs of an item at external metadata providers. */
export type ProviderIds = Schemas["ProviderIds"];

/** The selected artwork of an item, one image per {@link ArtworkKind}. */
export type Artwork = Schemas["Artwork"];

/** Kind of an artwork image. Later API versions may add values. */
export type ArtworkKind = Schemas["ArtworkKind"];

/** Reference to an artwork image; turn it into a URL only with `imageUrl`. */
export type ImageRef = Schemas["ImageRef"];

/** A width the server renders images in. Later API versions may add values. */
export type ImageWidth = Schemas["ImageWidth"];

/** Sort direction of a listing. Later API versions may add values. */
export type SortOrder = Schemas["SortOrder"];

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

/** One provider entry an item may be matched with; the result of an identify search. */
export type IdentifyCandidate = Schemas["IdentifyCandidate"];

/** Body of `POST /api/v1/items/{itemId}/identify/search`. */
export type IdentifySearchRequest = Schemas["IdentifySearchRequest"];

/** Whether an item is matched with an online provider. Later API versions may add values. */
export type MatchState = Schemas["MatchState"];

/** An online metadata provider. Later API versions may add values. */
export type MetadataProvider = Schemas["MetadataProvider"];

/** Sort key of a library listing. Later API versions may add values. */
export type ItemSort = Schemas["ItemSort"];

/** Quick filter of a library listing. Later API versions may add values. */
export type ItemQuickFilter = Schemas["ItemQuickFilter"];

/** Query parameters of `GET /api/v1/libraries/{libraryId}/items`: sort, filters and paging. */
export type LibraryItemsQuery = NonNullable<operations["listLibraryItems"]["parameters"]["query"]>;

/** Query parameters of `GET /api/v1/libraries/{libraryId}/letters`: order and filters. */
export type LibraryLettersQuery = NonNullable<
  operations["listLibraryLetters"]["parameters"]["query"]
>;

/** Where each first letter starts in the title-sorted list (`GET .../letters`). */
export type LetterIndex = Schemas["LetterIndex"];

/** One entry of a {@link LetterIndex}: a letter ("A"-"Z" or "#") and its first position. */
export type LetterOffset = Schemas["LetterOffset"];

/** Query parameters of `GET /api/v1/libraries/{libraryId}/facets`: fields and filters. */
export type LibraryFacetsQuery = NonNullable<
  operations["listLibraryFacets"]["parameters"]["query"]
>;

/** A filter field facet counts are given for. Later API versions may add values. */
export type FacetField = Schemas["FacetField"];

/** One value of a filter field with its item count. */
export type FacetValue = Schemas["FacetValue"];

/** The counted values of one filter field. */
export type Facet = Schemas["Facet"];

/** Facet counts of a library listing; the body of `GET /api/v1/libraries/{libraryId}/facets`. */
export type FacetList = Schemas["FacetList"];

/** One row of Home or of a library's Overview pivot with its first items. */
export type Row = Schemas["Row"];

/** What a row shows. Later API versions may add values; render unknown kinds generically. */
export type RowKind = Schemas["RowKind"];

/** Card shape of a row. Later API versions may add values; render unknown ones as posters. */
export type CardFormat = Schemas["CardFormat"];

/** Why a rows response has no or few rows. Later API versions may add values. */
export type RowsHint = Schemas["RowsHint"];

/** Sort and filters of a Browse pivot listing, as "See all" of a row opens it. */
export type BrowseQuery = Schemas["BrowseQuery"];

/** One title of the Home hero. */
export type HeroItem = Schemas["HeroItem"];

/** Body of `GET /api/v1/home/rows`: rows and hero of the Home screen. */
export type HomeRows = Schemas["HomeRows"];

/** Body of `GET /api/v1/libraries/{libraryId}/rows`: the rows of a library's Overview. */
export type LibraryRows = Schemas["LibraryRows"];

/** Body of `GET /api/v1/setup`: whether the server still waits for its first account. */
export type SetupState = Schemas["SetupState"];

/** Body of `POST /api/v1/setup`: the first administrator account. */
export type SetupRequest = Schemas["SetupRequest"];

/** Body of `POST /api/v1/auth/session`: name, password and the signing-in client. */
export type SignInRequest = Schemas["SignInRequest"];

/** A new session; for client "web" the cookie carries it and `token` is absent. */
export type Session = Schemas["Session"];

/** Kind of client that signs in. Later API versions may add values. */
export type ClientKind = Schemas["ClientKind"];

/** A local account. */
export type Account = Schemas["Account"];

/** Role of an account. Later API versions may add values. */
export type AccountRole = Schemas["AccountRole"];

/** A profile of the signed-in account as the profile picker shows it. */
export type ProfileSummary = Schemas["ProfileSummary"];

/** Token colour of a profile avatar. Later API versions may add values. */
export type AvatarColor = Schemas["AvatarColor"];

/** Body of `GET /api/v1/me`: the signed-in account, its selected profile and its profiles. */
export type Me = Schemas["Me"];

/** Body of `PUT /api/v1/me/password`. */
export type PasswordChange = Schemas["PasswordChange"];

/** Body of `PUT /api/v1/items/{itemId}/progress`: the absolute playback position in seconds. */
export type ProgressReport = Schemas["ProgressReport"];

/** The selected profile's watch state of an item; absent on items without state. */
export type ViewerState = Schemas["ViewerState"];

/** Where a device was last seen from. Later API versions may add values. */
export type AddressClass = Schemas["AddressClass"];

/** A signed-in device of the account, that is one of its sessions. */
export type Device = Schemas["Device"];

/** Body of `GET /api/v1/me/devices`. */
export type DeviceList = Schemas["DeviceList"];

/** A signed-in device of any account, with its account. */
export type AccountDevice = Schemas["AccountDevice"];

/** Body of `GET /api/v1/devices`. */
export type AccountDeviceList = Schemas["AccountDeviceList"];
/** An account as the user administration shows it (US-20). */
export type User = Schemas["User"];

/** Body of `GET /api/v1/users`. */
export type UserList = Schemas["UserList"];

/** Body of `POST /api/v1/users`: a new account with its initial password. */
export type UserCreate = Schemas["UserCreate"];

/** Body of `PATCH /api/v1/users/{userId}`; omitted properties stay as they are. */
export type UserUpdate = Schemas["UserUpdate"];

/** Whether an account may sign in. Later API versions may add values. */
export type UserStatus = Schemas["UserStatus"];

/** Body of `PUT /api/v1/users/{userId}/password`. */
export type PasswordReset = Schemas["PasswordReset"];

/** A profile with its settings as the account owner and the administration manage it. */
export type Profile = Schemas["Profile"];

/** Body of `POST /api/v1/users/{userId}/profiles`: a new managed profile. */
export type ProfileCreate = Schemas["ProfileCreate"];

/** Body of `PATCH /api/v1/profiles/{profileId}`; omitted properties stay, a null pin removes the PIN. */
export type ProfileUpdate = Schemas["ProfileUpdate"];

/** Body of `GET /api/v1/users/{userId}/profiles`. */
export type ProfileList = Schemas["ProfileList"];

/** The content limit of a profile; `preset` is derived from `maxAge` in responses. */
export type ContentLimit = Schemas["ContentLimit"];

/** Label of a content limit. Later API versions may add values. */
export type ContentLimitPreset = Schemas["ContentLimitPreset"];

/** Age bucket of a content limit. Later API versions may add values. */
export type AgeRating = Schemas["AgeRating"];

/** The libraries an account or profile may see. */
export type LibraryAccess = Schemas["LibraryAccess"];

/** A subtitle a player can show: an embedded stream (`s:<index>`) or an external file (`x:<hash>`). */
export type SubtitleTrack = Schemas["SubtitleTrack"];

/** What a session does with subtitles; a session without it shows none. */
export type SubtitleDecision = Schemas["SubtitleDecision"];

/** Off, sidecar (WebVTT) or burn-in. Later API versions may add values. */
export type SubtitleDeliveryMode = Schemas["SubtitleDeliveryMode"];

/** When subtitles switch on without a choice of the viewer. Later API versions may add values. */
export type SubtitlePreferenceMode = Schemas["SubtitlePreferenceMode"];
