// Package images serves artwork in fixed widths (GET /api/v1/images/{imageId}, ADR 0027 §8).
// The handler is a plain net/http handler mounted on the REST API mux (restapi.Mount), like
// the stream routes, so http.ServeContent can answer conditional and Range requests. Clients
// build the URL from an ImageRef; the image service renders and caches the pictures.
//
// The route is covered by the LAN-only guard like every API route. Authentication for images
// (cookie or signed URL) arrives in milestone 7.
package images
