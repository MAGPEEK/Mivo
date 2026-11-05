package artworkfetch

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

// ArtworkDirectory is the folder below the data directory that holds provider pictures;
// stored data paths start with it.
const ArtworkDirectory = "artwork"

const (
	directoryMode  = 0o750
	fileMode       = 0o640
	tempNameBytes  = 8
	tempFilePrefix = ".download-"
	tempFileSuffix = ".tmp"
	lowerHexDigits = "0123456789abcdef"
)

// isStoredDataPath accepts exactly the data paths this package writes, so remove can never
// be steered to another file.
func isStoredDataPath(dataPath string) bool {
	directory, name := path.Split(dataPath)
	if directory != ArtworkDirectory+"/" {
		return false
	}

	tag, extension, found := strings.Cut(name, ".")

	return found && isTag(tag) && slices.Contains(storedExtensions(), extension)
}

// isTag reports a lowercase hex string of artwork.TagLength digits.
func isTag(text string) bool {
	if len(text) != artwork.TagLength {
		return false
	}

	for _, digit := range text {
		if !strings.ContainsRune(lowerHexDigits, digit) {
			return false
		}
	}

	return true
}

// storedFile is a picture in the data directory.
type storedFile struct {
	DataPath   string
	Size       int64
	ModifiedAt time.Time
}

// pictureStore writes and removes pictures below <data>/artwork. Every access goes through
// an os.Root, so no path can leave the data directory even if a check above were wrong.
type pictureStore struct {
	dataDir string
}

func dataPathFor(tag, extension string) string {
	return path.Join(ArtworkDirectory, tag+"."+extension)
}

// find returns the stored picture of tag in any accepted format.
func (store pictureStore) find(tag string) (storedFile, bool, error) {
	root, err := os.OpenRoot(store.dataDir)
	if err != nil {
		return storedFile{}, false, fmt.Errorf("open data directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	for _, extension := range storedExtensions() {
		dataPath := dataPathFor(tag, extension)

		info, err := root.Stat(dataPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		if err != nil {
			return storedFile{}, false, fmt.Errorf("stat %s: %w", dataPath, err)
		}

		if info.Mode().IsRegular() {
			return storedFile{DataPath: dataPath, Size: info.Size(), ModifiedAt: info.ModTime()}, true, nil
		}
	}

	return storedFile{}, false, nil
}

// write stores body as <tag>.<extension> via a temporary file and a rename, so a reader
// never sees a partial picture.
func (store pictureStore) write(tag, extension string, body []byte) (storedFile, error) {
	root, err := os.OpenRoot(store.dataDir)
	if err != nil {
		return storedFile{}, fmt.Errorf("open data directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	if err := root.MkdirAll(ArtworkDirectory, directoryMode); err != nil {
		return storedFile{}, fmt.Errorf("create %s: %w", ArtworkDirectory, err)
	}

	tempPath, err := writeTemp(root, body)
	if err != nil {
		return storedFile{}, err
	}

	dataPath := dataPathFor(tag, extension)
	if err := root.Rename(tempPath, dataPath); err != nil {
		_ = root.Remove(tempPath)

		return storedFile{}, fmt.Errorf("move picture to %s: %w", dataPath, err)
	}

	info, err := root.Stat(dataPath)
	if err != nil {
		return storedFile{}, fmt.Errorf("stat %s: %w", dataPath, err)
	}

	return storedFile{DataPath: dataPath, Size: info.Size(), ModifiedAt: info.ModTime()}, nil
}

func writeTemp(root *os.Root, body []byte) (string, error) {
	random := make([]byte, tempNameBytes)
	_, _ = rand.Read(random) // never fails (crypto/rand documentation)
	tempPath := path.Join(ArtworkDirectory, tempFilePrefix+hex.EncodeToString(random)+tempFileSuffix)

	file, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return "", fmt.Errorf("create temporary picture: %w", err)
	}

	_, writeErr := file.Write(body)
	if writeErr == nil {
		writeErr = file.Sync()
	}

	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		_ = root.Remove(tempPath)

		return "", fmt.Errorf("write temporary picture: %w", err)
	}

	return tempPath, nil
}

// remove deletes stored pictures; missing files are not an error, so a repeated "Remove
// match" succeeds.
func (store pictureStore) remove(dataPaths []string) error {
	for _, dataPath := range dataPaths {
		if !isStoredDataPath(dataPath) {
			return fmt.Errorf("remove %q: not a provider picture path: %w", dataPath, domain.ErrInvalid)
		}
	}

	root, err := os.OpenRoot(store.dataDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("open data directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	for _, dataPath := range dataPaths {
		if err := root.Remove(dataPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", dataPath, err)
		}
	}

	return nil
}
