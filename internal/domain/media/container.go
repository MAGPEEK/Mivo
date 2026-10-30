package media

import (
	"fmt"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// Container is Mivo's short name of a file's container format (ADR 0023). The values are
// shared by the database, the API, the client profiles and the test corpus manifest.
type Container string

// The containers Mivo distinguishes. ffprobe reports whole families under one format name,
// so IdentifyContainer refines them with the file extension.
const (
	ContainerUnknown Container = "unknown"
	ContainerMKV     Container = "mkv"
	ContainerWebM    Container = "webm"
	ContainerMP4     Container = "mp4"
	ContainerM4V     Container = "m4v"
	ContainerMOV     Container = "mov"
	Container3GP     Container = "3gp"
	ContainerAVI     Container = "avi"
	ContainerMPEGTS  Container = "mpegts"
	ContainerM2TS    Container = "m2ts"
	ContainerFLV     Container = "flv"
	ContainerOGG     Container = "ogg"
	ContainerMPEG    Container = "mpeg"
	ContainerVOB     Container = "vob"
	ContainerASF     Container = "asf"
	ContainerMXF     Container = "mxf"
	ContainerRM      Container = "rm"
)

// Containers returns every known container except ContainerUnknown, in the order of
// ADR 0023. The caller owns the returned slice.
func Containers() []Container {
	return []Container{
		ContainerMKV, ContainerWebM, ContainerMP4, ContainerM4V, ContainerMOV, Container3GP,
		ContainerAVI, ContainerMPEGTS, ContainerM2TS, ContainerFLV, ContainerOGG, ContainerMPEG,
		ContainerVOB, ContainerASF, ContainerMXF, ContainerRM,
	}
}

// ParseContainer accepts the exact short name of a known container or "unknown", for
// example when reading a stored value back. Anything else wraps domain.ErrInvalid and
// returns ContainerUnknown.
func ParseContainer(text string) (Container, error) {
	container := Container(text)
	if container == ContainerUnknown || slices.Contains(Containers(), container) {
		return container, nil
	}

	return ContainerUnknown, fmt.Errorf("parse container %q: %w", text, domain.ErrInvalid)
}

// containerFamily groups the containers that ffprobe reports under one format name.
type containerFamily int

const (
	familyUnknown containerFamily = iota
	familyMatroska
	familyISOMedia
	familyTransportStream
	familyProgramStream
	familyVOB
	familyAVI
	familyFLV
	familyOGG
	familyASF
	familyMXF
	familyRealMedia
)

// IdentifyContainer maps ffprobe's format_name plus the file extension (with or without the
// leading dot, any case) to a Container. The format name decides the family because it
// reflects the bytes of the file; the extension only picks a member inside the family, so a
// misnamed file is never identified by its name alone. Unknown input yields
// ContainerUnknown.
func IdentifyContainer(formatName, extension string) Container {
	normalisedExtension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), "."))

	return familyMember(identifyFamily(formatName), normalisedExtension)
}

// identifyFamily returns the family of the first token of the comma-separated format name
// that Mivo knows.
func identifyFamily(formatName string) containerFamily {
	for token := range strings.SplitSeq(strings.ToLower(formatName), ",") {
		if family := familyOfFormat(strings.TrimSpace(token)); family != familyUnknown {
			return family
		}
	}

	return familyUnknown
}

func familyOfFormat(token string) containerFamily {
	switch token {
	case "matroska", "webm":
		return familyMatroska
	case "mov", "mp4", "m4a", "3gp", "3g2", "mj2":
		return familyISOMedia
	case "mpegts":
		return familyTransportStream
	case "mpeg":
		return familyProgramStream
	case "vob":
		return familyVOB
	case "avi":
		return familyAVI
	case "flv":
		return familyFLV
	case "ogg":
		return familyOGG
	case "asf":
		return familyASF
	case "mxf":
		return familyMXF
	case "rm":
		return familyRealMedia
	default:
		return familyUnknown
	}
}

// familyMember picks the container inside a family by extension. Families with a single
// member ignore the extension (.rmvb is rm, .wmv is asf, .divx is avi, .ogv is ogg).
func familyMember(family containerFamily, extension string) Container {
	switch family {
	case familyMatroska:
		return matroskaMember(extension)
	case familyISOMedia:
		return isoMediaMember(extension)
	case familyTransportStream:
		return transportStreamMember(extension)
	case familyProgramStream:
		return programStreamMember(extension)
	case familyVOB:
		return ContainerVOB
	case familyAVI:
		return ContainerAVI
	case familyFLV:
		return ContainerFLV
	case familyOGG:
		return ContainerOGG
	case familyASF:
		return ContainerASF
	case familyMXF:
		return ContainerMXF
	case familyRealMedia:
		return ContainerRM
	case familyUnknown:
		// Not a container Mivo knows; handled below together with foreign values.
	}

	return ContainerUnknown
}

// matroskaMember: WebM is a Matroska subset that only the extension tells apart.
func matroskaMember(extension string) Container {
	if extension == "webm" {
		return ContainerWebM
	}

	return ContainerMKV
}

// isoMediaMember: ffprobe reports every ISO base media file the same way. Without a known
// extension the file is treated as MP4, the most common and most widely playable member.
// 3G2 is grouped with 3GP because both share the codec set browsers care about.
func isoMediaMember(extension string) Container {
	switch extension {
	case "m4v":
		return ContainerM4V
	case "mov":
		return ContainerMOV
	case "3gp", "3g2":
		return Container3GP
	default:
		return ContainerMP4
	}
}

// transportStreamMember: Blu-ray (.m2ts) and AVCHD (.mts) use 192-byte packets, which
// ffprobe still reports as mpegts.
func transportStreamMember(extension string) Container {
	switch extension {
	case "m2ts", "mts":
		return ContainerM2TS
	default:
		return ContainerMPEGTS
	}
}

// programStreamMember: DVD .vob files are MPEG program streams for ffprobe.
func programStreamMember(extension string) Container {
	if extension == "vob" {
		return ContainerVOB
	}

	return ContainerMPEG
}
