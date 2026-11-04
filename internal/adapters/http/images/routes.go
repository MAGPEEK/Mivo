package images

import "net/http"

const (
	imageIDParameter = "imageId"
	// imagePattern serves one rendered image; GET also matches HEAD.
	imagePattern = http.MethodGet + " /api/v1/images/{" + imageIDParameter + "}"
)

// QuietPatterns returns the pattern of the image route for server.Options.QuietPatterns:
// a grid requests dozens of images at once, so successful requests are logged at debug level.
func QuietPatterns() []string {
	return []string{imagePattern}
}
