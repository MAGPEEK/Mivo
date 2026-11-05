package library

// Resolution is the resolution class of a video (API VideoResolution).
type Resolution string

// The resolution classes, lowest first.
const (
	ResolutionSD    Resolution = "sd"
	Resolution720p  Resolution = "720p"
	Resolution1080p Resolution = "1080p"
	Resolution4K    Resolution = "4k"
	Resolution8K    Resolution = "8k"
)

// Minimum width or height of each class above SD. A class is decided by width or height,
// so cropped (2.39:1) and portrait video land in the expected class.
const (
	minWidth8K     = 7000
	minHeight8K    = 4000
	minWidth4K     = 3200
	minHeight4K    = 1800
	minWidth1080p  = 1600
	minHeight1080p = 900
	minWidth720p   = 1100
	minHeight720p  = 650
)

// ResolutionThreshold is the lower bound of a resolution class: a video belongs to the
// highest class whose MinWidth or MinHeight it reaches.
type ResolutionThreshold struct {
	Resolution Resolution
	MinWidth   int
	MinHeight  int
}

// ResolutionThresholds returns the bounds of the classes above SD, highest first. Video
// below every bound is SD. Adapters build their SQL from it, so the rule lives here once.
func ResolutionThresholds() []ResolutionThreshold {
	return []ResolutionThreshold{
		{Resolution: Resolution8K, MinWidth: minWidth8K, MinHeight: minHeight8K},
		{Resolution: Resolution4K, MinWidth: minWidth4K, MinHeight: minHeight4K},
		{Resolution: Resolution1080p, MinWidth: minWidth1080p, MinHeight: minHeight1080p},
		{Resolution: Resolution720p, MinWidth: minWidth720p, MinHeight: minHeight720p},
	}
}

// Resolutions returns every class, lowest first: the order of the resolution facet.
func Resolutions() []Resolution {
	return []Resolution{ResolutionSD, Resolution720p, Resolution1080p, Resolution4K, Resolution8K}
}

// ResolutionOf classifies a video by its pixel size.
func ResolutionOf(width, height int) Resolution {
	for _, threshold := range ResolutionThresholds() {
		if width >= threshold.MinWidth || height >= threshold.MinHeight {
			return threshold.Resolution
		}
	}

	return ResolutionSD
}

// IsValid reports whether the class is one of the declared values.
func (resolution Resolution) IsValid() bool {
	for _, known := range Resolutions() {
		if resolution == known {
			return true
		}
	}

	return false
}

// IsUHD reports whether the class is 4K or 8K, the quick filter "uhd".
func (resolution Resolution) IsUHD() bool {
	return resolution == Resolution4K || resolution == Resolution8K
}
