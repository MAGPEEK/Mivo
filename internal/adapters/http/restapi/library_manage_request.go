package restapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/library"
)

// createRequest converts the body of createLibrary. Missing optional values stay empty or
// nil, which the manager reads as the server defaults.
func createRequest(body apigen.LibraryCreate) applibrary.CreateRequest {
	request := applibrary.CreateRequest{
		Name:    body.Name,
		Kind:    library.Kind(body.Kind),
		Folders: folderRefs(body.Folders),
		Options: optionsRequest(body.Options),
	}

	if body.MetadataLanguage != nil {
		request.MetadataLanguage = *body.MetadataLanguage
	}

	return request
}

// updatePatch converts the body of updateLibrary; omitted properties stay nil.
func updatePatch(body apigen.LibraryUpdate) applibrary.UpdatePatch {
	patch := applibrary.UpdatePatch{
		Name:             body.Name,
		MetadataLanguage: body.MetadataLanguage,
		Options:          optionsRequest(body.Options),
	}

	if body.Folders != nil {
		refs := folderRefs(*body.Folders)
		patch.Folders = &refs
	}

	return patch
}

func folderRefs(folders []apigen.FolderRef) []applibrary.FolderRef {
	refs := make([]applibrary.FolderRef, 0, len(folders))

	for _, folder := range folders {
		ref := applibrary.FolderRef{}
		if folder.FolderID != nil {
			ref.FolderID = folder.FolderID.String()
		}

		if folder.RootID != nil {
			ref.RootID = folder.RootID.String()
		}

		if folder.Path != nil {
			ref.Path = *folder.Path
		}

		refs = append(refs, ref)
	}

	return refs
}

func optionsRequest(options *apigen.LibraryOptions) *library.Options {
	if options == nil {
		return nil
	}

	return &library.Options{
		ScanExtras: options.ScanExtras, ReadNFO: options.ReadNfo, LocalArtwork: options.LocalArtwork,
	}
}

// The strict body shapes of the management requests. The generated decoder ignores unknown
// properties and cannot tell a missing boolean from false, so checkLibraryBody decodes the
// raw body once more into these types, which reject both.
type (
	strictFolderRef struct {
		FolderID *string `json:"folderId"`
		RootID   *string `json:"rootId"`
		Path     *string `json:"path"`
	}
	strictOptions struct {
		ScanExtras   *bool `json:"scanExtras"`
		ReadNfo      *bool `json:"readNfo"`
		LocalArtwork *bool `json:"localArtwork"`
		// OnlineMetadata is optional (ADR 0031 section 2, on when omitted). It is accepted
		// but not persisted yet: storage arrives with task M6-03.
		OnlineMetadata *bool `json:"onlineMetadata"`
	}
	strictCreate struct {
		Kind             *string           `json:"kind"`
		Name             *string           `json:"name"`
		Folders          []strictFolderRef `json:"folders"`
		MetadataLanguage *string           `json:"metadataLanguage"`
		Options          *strictOptions    `json:"options"`
	}
	// strictUpdate has no kind: a body that names it fails as an unknown property.
	strictUpdate struct {
		Name             *string            `json:"name"`
		Folders          *[]strictFolderRef `json:"folders"`
		MetadataLanguage *string            `json:"metadataLanguage"`
		Options          *strictOptions     `json:"options"`
	}
)

var errBodyShape = errors.New("request body does not match the contract")

// checkLibraryBody rejects what the generated decoder lets through: unknown properties
// (additionalProperties: false, so also the kind in an update), options without all three
// switches and an update without any property.
func checkLibraryBody(method string, raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var options *strictOptions

	if method == http.MethodPost {
		var body strictCreate
		if err := decoder.Decode(&body); err != nil {
			return fmt.Errorf("%w: %w", errBodyShape, err)
		}

		options = body.Options
	} else {
		var body strictUpdate
		if err := decoder.Decode(&body); err != nil {
			return fmt.Errorf("%w: %w", errBodyShape, err)
		}

		if body.Name == nil && body.Folders == nil && body.MetadataLanguage == nil && body.Options == nil {
			return fmt.Errorf("%w: no property to change", errBodyShape)
		}

		options = body.Options
	}

	if options != nil && (options.ScanExtras == nil || options.ReadNfo == nil || options.LocalArtwork == nil) {
		return fmt.Errorf("%w: options need scanExtras, readNfo and localArtwork", errBodyShape)
	}

	return nil
}

// isLibraryBodyRequest reports the two requests with a library body: POST of the collection
// and PATCH of one library.
func isLibraryBodyRequest(request *http.Request) bool {
	switch request.Method {
	case http.MethodPost:
		return request.URL.Path == librariesPath
	case http.MethodPatch:
		rest, found := strings.CutPrefix(request.URL.Path, librariesPath+"/")

		return found && rest != "" && !strings.Contains(rest, "/")
	default:
		return false
	}
}

// checkLibraryBodies validates the bodies of createLibrary and updateLibrary before the
// generated handler sees them and hands the same bytes on. Other requests pass untouched.
func (api *API) checkLibraryBodies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !isLibraryBodyRequest(request) {
			next.ServeHTTP(writer, request)

			return
		}

		raw, err := io.ReadAll(request.Body)
		if err == nil {
			err = checkLibraryBody(request.Method, raw)
		}

		if err != nil {
			api.writeRequestError(writer, request, err)

			return
		}

		request.Body = io.NopCloser(bytes.NewReader(raw))
		next.ServeHTTP(writer, request)
	})
}
