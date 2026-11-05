package library_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const folderID = "6a7b8c9d-0e1f-4a2b-8c3d-4e5f6a7b8c9d"

func TestParseFolderPathAcceptsPlainPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want library.FolderPath
	}{
		{text: "", want: library.FolderPath{}},
		{text: folderID, want: library.FolderPath{FolderID: folderID, Segments: []string{}}},
		{text: folderID + "/Animation/Up (2009)", want: library.FolderPath{FolderID: folderID, Segments: []string{"Animation", "Up (2009)"}}},
		{text: folderID + "/Über Filme", want: library.FolderPath{FolderID: folderID, Segments: []string{"Über Filme"}}},
		{text: folderID + "/console", want: library.FolderPath{FolderID: folderID, Segments: []string{"console"}}},
		{text: folderID + "/COM10", want: library.FolderPath{FolderID: folderID, Segments: []string{"COM10"}}},
		{text: folderID + "/.hidden", want: library.FolderPath{FolderID: folderID, Segments: []string{".hidden"}}},
	}
	for _, test := range tests {
		got, err := library.ParseFolderPath(test.text)
		if err != nil {
			t.Errorf("ParseFolderPath(%q) error = %v", test.text, err)
			continue
		}

		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("ParseFolderPath(%q) = %+v, want %+v", test.text, got, test.want)
		}

		if got.String() != test.text {
			t.Errorf("ParseFolderPath(%q).String() = %q", test.text, got.String())
		}
	}
}

func TestParseFolderPathRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"leading slash":       "/" + folderID,
		"absolute unix":       "/etc/passwd",
		"backslash":           folderID + `\a`,
		"parent segment":      folderID + "/../x",
		"current segment":     folderID + "/./x",
		"empty segment":       folderID + "//x",
		"trailing slash":      folderID + "/x/",
		"only parent":         "..",
		"nul":                 folderID + "/a\x00b",
		"control character":   folderID + "/a\x1fb",
		"delete character":    folderID + "/a\x7fb",
		"c1 control":          folderID + "/a\u0085b",
		"invalid utf-8":       folderID + "/a\xffb",
		"colon":               folderID + "/a:b",
		"drive letter":        "C:/x",
		"reserved con":        folderID + "/CON",
		"reserved lowercase":  folderID + "/nul",
		"reserved extension":  folderID + "/aux.txt",
		"reserved with space": folderID + "/prn .txt",
		"reserved com":        folderID + "/com1",
		"reserved lpt":        folderID + "/LPT9.log",
		"reserved super":      folderID + "/COM¹",
		"too long":            folderID + "/" + strings.Repeat("a", library.MaxFolderPathLength),
	}
	for name, text := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := library.ParseFolderPath(text)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("ParseFolderPath(%q) error = %v, want domain.ErrInvalid", text, err)
			}
		})
	}
}

func TestFolderPathNavigation(t *testing.T) {
	t.Parallel()

	top := library.FolderPath{}
	if !top.IsTop() || top.String() != "" || top.Relative() != "" {
		t.Errorf("top = %+v, want IsTop and empty texts", top)
	}

	if _, found := top.Parent(); found {
		t.Error("top.Parent() found a parent")
	}

	root := top.Child(folderID)
	if root.String() != folderID || root.Relative() != "" {
		t.Errorf("top.Child = %q (%q), want the folder id", root.String(), root.Relative())
	}

	deep := root.Child("Animation").Child("Up")
	if deep.String() != folderID+"/Animation/Up" || deep.Relative() != "Animation/Up" {
		t.Errorf("deep = %q (%q)", deep.String(), deep.Relative())
	}

	parent, found := deep.Parent()
	if !found || parent.String() != folderID+"/Animation" {
		t.Errorf("deep.Parent() = %q, %v", parent.String(), found)
	}

	rootParent, found := root.Parent()
	if !found || !rootParent.IsTop() {
		t.Errorf("root.Parent() = %+v, %v, want the top", rootParent, found)
	}

	if root.Child("x").String() != folderID+"/x" || len(root.Segments) != 0 {
		t.Error("Child changed its receiver")
	}
}
