package media_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
)

func TestDirectPlayKeyFollowsADR0023(t *testing.T) {
	t.Parallel()

	tests := []struct {
		container media.Container
		wantKey   media.DirectPlayContainer
		wantOK    bool
	}{
		{container: media.ContainerMP4, wantKey: media.DirectPlayMP4, wantOK: true},
		{container: media.ContainerM4V, wantKey: media.DirectPlayMP4, wantOK: true},
		{container: media.ContainerMOV, wantKey: media.DirectPlayMOV, wantOK: true},
		{container: media.ContainerWebM, wantKey: media.DirectPlayWebM, wantOK: true},
		{container: media.ContainerMKV, wantKey: media.DirectPlayMKV, wantOK: true},
		{container: media.ContainerOGG, wantKey: media.DirectPlayOGG, wantOK: true},
		{container: media.ContainerMPEGTS, wantKey: media.DirectPlayMPEGTS, wantOK: true},
		{container: media.ContainerM2TS, wantKey: media.DirectPlayMPEGTS, wantOK: true},
		{container: media.Container3GP, wantKey: media.DirectPlay3GP, wantOK: true},
		{container: media.ContainerAVI},
		{container: media.ContainerFLV},
		{container: media.ContainerMPEG},
		{container: media.ContainerVOB},
		{container: media.ContainerASF},
		{container: media.ContainerMXF},
		{container: media.ContainerRM},
		{container: media.ContainerUnknown},
		{container: media.Container("")},
		{container: media.Container("future")},
	}

	for _, test := range tests {
		t.Run(string(test.container), func(t *testing.T) {
			t.Parallel()

			key, ok := media.DirectPlayKey(test.container)
			if key != test.wantKey || ok != test.wantOK {
				t.Errorf("DirectPlayKey(%q) = (%q, %t), want (%q, %t)",
					test.container, key, ok, test.wantKey, test.wantOK)
			}
		})
	}
}

func TestMIMETypeFollowsADR0023(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key      media.DirectPlayContainer
		wantMIME string
		wantOK   bool
	}{
		{key: media.DirectPlayMP4, wantMIME: "video/mp4", wantOK: true},
		{key: media.DirectPlayMOV, wantMIME: "video/quicktime", wantOK: true},
		{key: media.DirectPlayWebM, wantMIME: "video/webm", wantOK: true},
		{key: media.DirectPlayMKV, wantMIME: "video/x-matroska", wantOK: true},
		{key: media.DirectPlayOGG, wantMIME: "video/ogg", wantOK: true},
		{key: media.DirectPlayMPEGTS, wantMIME: "video/mp2t", wantOK: true},
		{key: media.DirectPlay3GP, wantMIME: "video/3gpp", wantOK: true},
		{key: media.DirectPlayContainer("")},
		{key: media.DirectPlayContainer("avi")},
		{key: media.DirectPlayContainer("m4v")},
		{key: media.DirectPlayContainer("MP4")},
	}

	for _, test := range tests {
		t.Run(string(test.key), func(t *testing.T) {
			t.Parallel()

			mimeType, ok := media.MIMEType(test.key)
			if mimeType != test.wantMIME || ok != test.wantOK {
				t.Errorf("MIMEType(%q) = (%q, %t), want (%q, %t)",
					test.key, mimeType, ok, test.wantMIME, test.wantOK)
			}
		})
	}
}

func TestEveryDirectPlayKeyHasAMIMEType(t *testing.T) {
	t.Parallel()

	for _, container := range media.Containers() {
		key, ok := media.DirectPlayKey(container)
		if !ok {
			continue
		}

		if _, hasMIME := media.MIMEType(key); !hasMIME {
			t.Errorf("DirectPlayKey(%q) = %q has no MIME type", container, key)
		}
	}
}
