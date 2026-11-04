package library_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestAudiobookKinds(t *testing.T) {
	t.Parallel()

	if kind, err := library.ParseKind("audiobooks"); err != nil || kind != library.KindAudiobooks {
		t.Errorf("ParseKind(audiobooks) = %q, %v", kind, err)
	}

	if item, err := library.ParseItemKind("audiobook"); err != nil || item != library.ItemKindAudiobook {
		t.Errorf("ParseItemKind(audiobook) = %q, %v", item, err)
	}

	got, err := library.ItemKindForLibrary(library.KindAudiobooks)
	if err != nil || got != library.ItemKindAudiobook {
		t.Errorf("ItemKindForLibrary = %q, %v", got, err)
	}

	if !library.ItemKindAudiobook.OwnsFiles() {
		t.Error("audiobook must own files")
	}

	if !library.ValidParent(library.ItemKindAudiobook, "") {
		t.Error("audiobook must be top level")
	}

	if library.ValidParent(library.ItemKindAudiobook, library.ItemKindShow) {
		t.Error("audiobook must not sit below a show")
	}
}
