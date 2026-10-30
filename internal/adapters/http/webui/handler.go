// Package webui serves the browser app from an fs.FS (ADR 0020). Builds with the webui tag
// embed the Vite output from dist; plain builds serve a placeholder page that explains how to
// include the app. Both variants share this handler, only Files differs.
package webui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

const (
	// indexFile is the SPA entry point, served for every client route.
	indexFile = "index.html"

	// apiRootName and apiNamePrefix are the API subtree as file names (without the leading
	// slash). The web UI never answers there, not even with index.html.
	apiRootName   = "api"
	apiNamePrefix = apiRootName + "/"

	// hashedAssetsPrefix is the directory Vite writes content-hashed files to. A changed file
	// gets a new name, so these can be cached forever.
	hashedAssetsPrefix = "assets/"

	// cacheForever lets browsers and proxies keep hashed assets for a year without asking.
	cacheForever = "public, max-age=31536000, immutable"

	// cacheRevalidate makes browsers ask again before reusing a file whose name stays the same
	// across releases, above all index.html, which names the current hashed assets.
	cacheRevalidate = "no-cache"

	// allowedMethods is the Allow header of a 405 answer.
	allowedMethods = "GET, HEAD"
)

// Handler serves the files of the web UI with explicit content types and cache headers. Paths
// without a file extension that match no file get index.html, so the client router can
// resolve them; directories are never listed. Paths below /api get a 404 problem.
type Handler struct {
	files  fs.FS
	logger *slog.Logger
}

// NewHandler returns a handler for files, whose root must contain index.html. A missing or
// unreadable index.html is an error, because every client route depends on it. logger
// receives the cause of every 500 answer; the client only sees the status.
func NewHandler(files fs.FS, logger *slog.Logger) (*Handler, error) {
	if files == nil {
		return nil, errors.New("web ui: no file system for " + indexFile)
	}

	if logger == nil {
		return nil, errors.New("web ui: missing logger")
	}

	info, err := fs.Stat(files, indexFile)
	if err != nil {
		return nil, fmt.Errorf("web ui: %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("web ui: %s is not a regular file", indexFile)
	}

	return &Handler{files: files, logger: logger}, nil
}

// ServeHTTP answers GET and HEAD for the UI files. It is meant for every path outside /api/;
// the server routes API paths elsewhere, so unknown API routes never get index.html.
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", allowedMethods)
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

		return
	}

	name := fileName(request.URL.Path)
	if isAPIName(name) {
		// Defence in depth behind the server's routing: an API client asking for a missing
		// route must get the API's 404 problem, never index.html with status 200.
		restapi.WriteProblem(writer, restapi.NewProblem(http.StatusNotFound, restapi.CodeNotFound))

		return
	}

	opened, name, err := handler.open(name)

	switch {
	case errors.Is(err, fs.ErrNotExist):
		http.NotFound(writer, request)
	case err != nil:
		handler.serverError(writer, request, err)
	default:
		defer func() { _ = opened.file.Close() }() // read-only file, nothing is lost on a failed close

		handler.serveFile(writer, request, name, opened)
	}
}

// open opens the file for name, or index.html when name is a client route that matches no
// file. It returns the name of the file it opened.
func (handler *Handler) open(name string) (fileWithInfo, string, error) {
	file, info, err := openRegularFile(handler.files, name)
	if errors.Is(err, fs.ErrNotExist) && isClientRoute(name) {
		name = indexFile
		file, info, err = openRegularFile(handler.files, name)
	}

	return fileWithInfo{file: file, info: info}, name, err
}

// serverError logs err and answers 500 without its text, which may name internal paths.
func (handler *Handler) serverError(writer http.ResponseWriter, request *http.Request, err error) {
	handler.logger.ErrorContext(request.Context(), "web ui request failed",
		slog.String("method", request.Method),
		slog.String("path", request.URL.Path),
		slog.String("request_id", requestid.FromContext(request.Context())),
		slog.Any("error", err))
	http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// isAPIName reports whether a cleaned file name lies in the API subtree.
func isAPIName(name string) bool {
	return name == apiRootName || strings.HasPrefix(name, apiNamePrefix)
}

// fileName maps a URL path to a name inside the UI file system. Cleaning the rooted path
// resolves every ".." against the root, so a request can never name a file above it.
func fileName(urlPath string) string {
	name := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if name == "" {
		return indexFile
	}

	return name
}

// isClientRoute reports whether a missing name is a route of the client router rather than a
// missing file: routes have no file extension.
func isClientRoute(name string) bool {
	return path.Ext(name) == ""
}

// openRegularFile opens name and returns fs.ErrNotExist for directories, so they are handled
// like missing files and never listed.
func openRegularFile(files fs.FS, name string) (fs.File, fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, nil, fmt.Errorf("open %s: %w", name, fs.ErrNotExist)
	}

	file, err := files.Open(name)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", name, err)
	}

	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fs.ErrNotExist
	}

	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("stat %s: %w", name, err), file.Close())
	}

	return file, info, nil
}

// fileWithInfo is an open regular file with its metadata.
type fileWithInfo struct {
	file fs.File
	info fs.FileInfo
}

// serveFile sends file through http.ServeContent, which handles HEAD, ranges and conditional
// requests. The content type is set beforehand, so ServeContent never sniffs it.
func (handler *Handler) serveFile(writer http.ResponseWriter, request *http.Request, name string, opened fileWithInfo) {
	content, err := seekable(opened.file)
	if err != nil {
		handler.serverError(writer, request, fmt.Errorf("serve %s: %w", name, err))

		return
	}

	header := writer.Header()
	header.Set("Content-Type", contentTypeFor(name))
	header.Set("Cache-Control", cacheControlFor(name))

	http.ServeContent(writer, request, name, opened.info.ModTime(), content)
}

// seekable returns file itself when it can seek (embed.FS files can) and otherwise reads it
// into memory; UI files are small.
func seekable(file fs.File) (io.ReadSeeker, error) {
	if content, ok := file.(io.ReadSeeker); ok {
		return content, nil
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read web ui file: %w", err)
	}

	return bytes.NewReader(data), nil
}

func cacheControlFor(name string) string {
	if strings.HasPrefix(name, hashedAssetsPrefix) {
		return cacheForever
	}

	return cacheRevalidate
}

// contentTypeFor maps the extensions a Vite build produces to their media type. The table is
// explicit because Go's mime package reads the Windows registry, which may map .js to
// text/plain; browsers then refuse module scripts under X-Content-Type-Options: nosniff.
func contentTypeFor(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json", ".map":
		return "application/json"
	case ".webmanifest":
		return "application/manifest+json"
	case ".woff2":
		return "font/woff2"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	case ".txt":
		return "text/plain; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}
