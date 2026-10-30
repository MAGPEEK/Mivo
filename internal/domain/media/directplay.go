package media

// DirectPlayContainer is the key a client profile uses to say which progressive files it
// plays (ADR 0023, ClientProfile.directPlay[].container). Several containers share a key
// when browsers treat them alike, for example mp4 and m4v.
type DirectPlayContainer string

// The direct-play keys of ADR 0023.
const (
	DirectPlayMP4    DirectPlayContainer = "mp4"
	DirectPlayMOV    DirectPlayContainer = "mov"
	DirectPlayWebM   DirectPlayContainer = "webm"
	DirectPlayMKV    DirectPlayContainer = "mkv"
	DirectPlayOGG    DirectPlayContainer = "ogg"
	DirectPlayMPEGTS DirectPlayContainer = "mpegts"
	DirectPlay3GP    DirectPlayContainer = "3gp"
)

// DirectPlayKey returns the direct-play key of a container. The second result is false for
// containers that are never played directly (avi, flv, mpeg, vob, asf, mxf, rm and unknown):
// no browser plays them as a progressive file.
func DirectPlayKey(container Container) (DirectPlayContainer, bool) {
	switch container {
	case ContainerMP4, ContainerM4V:
		return DirectPlayMP4, true
	case ContainerMOV:
		return DirectPlayMOV, true
	case ContainerWebM:
		return DirectPlayWebM, true
	case ContainerMKV:
		return DirectPlayMKV, true
	case ContainerOGG:
		return DirectPlayOGG, true
	case ContainerMPEGTS, ContainerM2TS:
		return DirectPlayMPEGTS, true
	case Container3GP:
		return DirectPlay3GP, true
	case ContainerAVI, ContainerFLV, ContainerMPEG, ContainerVOB, ContainerASF, ContainerMXF,
		ContainerRM, ContainerUnknown:
		// Never played directly; listed so the exhaustive linter flags a new container.
	}

	return "", false
}

// MIMEType returns the MIME type of a direct-play key. It is used both by the browser to
// probe playability and as Content-Type when the file is served. The second result is false
// for anything that is not a direct-play key, so it also validates keys from client
// profiles.
func MIMEType(key DirectPlayContainer) (string, bool) {
	switch key {
	case DirectPlayMP4:
		return "video/mp4", true
	case DirectPlayMOV:
		return "video/quicktime", true
	case DirectPlayWebM:
		return "video/webm", true
	case DirectPlayMKV:
		return "video/x-matroska", true
	case DirectPlayOGG:
		return "video/ogg", true
	case DirectPlayMPEGTS:
		return "video/mp2t", true
	case DirectPlay3GP:
		return "video/3gpp", true
	default:
		return "", false
	}
}
