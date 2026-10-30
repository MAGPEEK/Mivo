package transcodedir_test

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestOpenFileReadsAFileOfTheSession(t *testing.T) {
	t.Parallel()

	workspace, _ := newWorkspace(t)

	dir, err := workspace.Create(t.Context(), firstSessionID)
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}

	writeFile(t, filepath.Join(dir, playlistName), playlistBody)

	file, err := workspace.OpenFile(firstSessionID, playlistName)
	if err != nil {
		t.Fatalf("OpenFile error = %v", err)
	}

	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil || string(content) != playlistBody {
		t.Errorf("read = (%q, %v), want (%q, nil)", content, err, playlistBody)
	}
}

func TestOpenFileRejectsNamesLeavingTheSession(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	sessions := filepath.Join(root, sessionsDirName)
	writeFile(t, filepath.Join(sessions, firstSessionID, playlistName), playlistBody)
	writeFile(t, filepath.Join(sessions, secondSessionID, playlistName), playlistBody)
	writeFile(t, filepath.Join(root, "secret.txt"), "secret")

	names := map[string]string{
		"sibling session":      filepath.Join("..", secondSessionID, playlistName),
		"transcode root":       filepath.Join("..", "..", "secret.txt"),
		"absolute":             filepath.Join(root, "secret.txt"),
		"absolute in session":  filepath.Join(sessions, firstSessionID, playlistName),
		"slash traversal":      "../" + secondSessionID + "/" + playlistName,
		"nested via traversal": "x/../../" + secondSessionID + "/" + playlistName,
	}

	for name, fileName := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			file, err := workspace.OpenFile(firstSessionID, fileName)
			if err == nil {
				_ = file.Close()
				t.Errorf("OpenFile(%q) error = nil, want an error", fileName)
			}
		})
	}
}

func TestOpenFileRejectsSymlinksLeavingTheSession(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	sessions := filepath.Join(root, sessionsDirName)
	writeFile(t, filepath.Join(sessions, secondSessionID, playlistName), playlistBody)
	writeFile(t, filepath.Join(root, "secret.txt"), "secret")
	mkdir(t, filepath.Join(sessions, firstSessionID))

	links := map[string]string{
		"relative sibling": filepath.Join("..", secondSessionID, playlistName),
		"relative root":    filepath.Join("..", "..", "secret.txt"),
		"absolute":         filepath.Join(root, "secret.txt"),
	}

	for name, target := range links {
		link := filepath.Join(sessions, firstSessionID, "link-"+filepath.Base(target)+"-"+name)
		symlinkOrSkip(t, target, link)

		file, err := workspace.OpenFile(firstSessionID, filepath.Base(link))
		if err == nil {
			_ = file.Close()
			t.Errorf("OpenFile through symlink %q error = nil, want an error", name)
		}
	}
}

func TestOpenFileOfAMissingSessionIsNotFound(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)

	if _, err := workspace.OpenFile(firstSessionID, playlistName); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("OpenFile without sessions directory error = %v, want ErrNotFound", err)
	}

	mkdir(t, filepath.Join(root, sessionsDirName, firstSessionID))

	if _, err := workspace.OpenFile(secondSessionID, playlistName); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("OpenFile of a missing session error = %v, want ErrNotFound", err)
	}

	if _, err := workspace.OpenFile(firstSessionID, playlistName); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("OpenFile of a missing file error = %v, want ErrNotFound", err)
	}
}

func TestOpenFileRejectsNonCanonicalSessionIDs(t *testing.T) {
	t.Parallel()

	workspace, root := newWorkspace(t)
	writeFile(t, filepath.Join(root, sessionsDirName, playlistName), playlistBody)

	for _, testCase := range invalidSessionIDs() {
		if _, err := workspace.OpenFile(testCase.id, playlistName); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("OpenFile(%q) error = %v, want ErrInvalid", testCase.id, err)
		}
	}
}
