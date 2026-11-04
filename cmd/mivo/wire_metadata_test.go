package main

import (
	"errors"
	"testing"

	appscan "github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
)

func TestNFOParserAdaptsTheReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		kind      appscan.NFOKind
		wantTitle string
		wantErr   error
	}{
		{name: "movie", content: "<movie><title>Heat</title></movie>", kind: appscan.NFOKindMovie, wantTitle: "Heat"},
		{name: "show", content: "<tvshow><title>Pioneer One</title></tvshow>", kind: appscan.NFOKindShow, wantTitle: "Pioneer One"},
		{name: "wrong root element", content: "<movie><title>Heat</title></movie>", kind: appscan.NFOKindShow, wantErr: domain.ErrUnsupported},
		{name: "unknown kind", content: "<movie/>", kind: appscan.NFOKind("album"), wantErr: domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := nfoParser{}.Parse([]byte(test.content), test.kind)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Parse error = %v, want %v", err, test.wantErr)
			}

			if test.wantErr == nil && (len(result.Records) != 1 || result.Records[0].Title != test.wantTitle) {
				t.Errorf("records = %+v, want one titled %q", result.Records, test.wantTitle)
			}
		})
	}
}
