package coverart

import "testing"

func TestSafeImageURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    string
		want   string
		wantOK bool
	}{
		{"https kept", "https://coverartarchive.org/release/a/1-500.jpg", "https://coverartarchive.org/release/a/1-500.jpg", true},
		{"http rewritten", "http://coverartarchive.org/release/a/1.jpg", "https://coverartarchive.org/release/a/1.jpg", true},
		{"host case normalised", "http://CoverArtArchive.org/release/a/1.jpg", "https://coverartarchive.org/release/a/1.jpg", true},
		{"empty", "", "", false},
		{"unparsable", "http://%zz", "", false},
		{"ftp", "ftp://coverartarchive.org/a.jpg", "", false},
		{"relative", "/release/a/1.jpg", "", false},
		{"archive mirror", "https://ia800.us.archive.org/a.jpg", "", false},
		{"lookalike", "https://coverartarchive.org.evil.example/a.jpg", "", false},
		{"port", "https://coverartarchive.org:8443/a.jpg", "", false},
		{"credentials", "https://user:pw@coverartarchive.org/a.jpg", "", false},
		{"query", "https://coverartarchive.org/a.jpg?x=1", "", false},
		{"fragment", "https://coverartarchive.org/a.jpg#x", "", false},
		{"escaped", "https://coverartarchive.org/a%2Fb.jpg", "", false},
		{"dot segment", "https://coverartarchive.org/release/../a.jpg", "", false},
		{"backslash", "https://coverartarchive.org/release/a\\b.jpg", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := safeImageURL(test.raw)
			if got != test.want || ok != test.wantOK {
				t.Errorf("safeImageURL(%q) = %q, %v; want %q, %v", test.raw, got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestNormalizedMBID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		id     string
		want   string
		wantOK bool
	}{
		{"valid", "76df3287-6cda-33eb-8e9a-044b5e15ffdd", "76df3287-6cda-33eb-8e9a-044b5e15ffdd", true},
		{"upper case", "76DF3287-6CDA-33EB-8E9A-044B5E15FFDD", "76df3287-6cda-33eb-8e9a-044b5e15ffdd", true},
		{"too short", "76df3287", "", false},
		{"missing hyphen", "76df32870-cda-33eb-8e9a-044b5e15ffdd", "", false},
		{"not hex", "76df3287-6cda-33eb-8e9a-044b5e15ffzz", "", false},
		{"slash", "76df3287-6cda-33eb-8e9a-044b5e15ff/d", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := normalizedMBID(test.id)
			if got != test.want || ok != test.wantOK {
				t.Errorf("normalizedMBID(%q) = %q, %v; want %q, %v", test.id, got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestFrontCoversCapsAtMaximum(t *testing.T) {
	t.Parallel()

	var listing imageListing

	for _, name := range []string{"1", "2", "3", "4", "5", "6", "7"} {
		listing.Images = append(listing.Images, coverImage{
			Approved: true, Front: true, Image: "https://coverartarchive.org/release/a/" + name + ".jpg",
		})
	}

	if got := frontCovers(listing); len(got) != maxCovers {
		t.Errorf("frontCovers() returned %d refs, want %d", len(got), maxCovers)
	}
}
