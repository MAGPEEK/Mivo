package restapi

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/app/auth"
)

func TestAuthDeviceLabelFromUserAgent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		userAgent    string
		wantName     string
		wantPlatform string
	}{
		{
			name:      "firefox on windows",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0",
			wantName:  "Firefox on Windows", wantPlatform: "Windows",
		},
		{
			name:      "chrome on macos",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
			wantName:  "Chrome on macOS", wantPlatform: "macOS",
		},
		{
			name:      "edge names chrome too",
			userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
			wantName:  "Edge on Windows", wantPlatform: "Windows",
		},
		{
			name:      "opera names chrome too",
			userAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 OPR/115.0.0.0",
			wantName:  "Opera on Linux", wantPlatform: "Linux",
		},
		{
			name:      "safari on iphone names mac os x",
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1",
			wantName:  "Safari on iOS", wantPlatform: "iOS",
		},
		{
			name:      "chrome on android names linux",
			userAgent: "Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
			wantName:  "Chrome on Android", wantPlatform: "Android",
		},
		{
			name:      "samsung internet",
			userAgent: "Mozilla/5.0 (Linux; Android 14; SM-S921B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36",
			wantName:  "Samsung Internet on Android", wantPlatform: "Android",
		},
		{
			name:      "lg webos tv",
			userAgent: "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/108.0.5359.211 Safari/537.36 WebAppManager",
			wantName:  "Chrome on webOS", wantPlatform: "webOS",
		},
		{
			name:      "chromeos",
			userAgent: "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
			wantName:  "Chrome on ChromeOS", wantPlatform: "ChromeOS",
		},
		{name: "curl has no platform", userAgent: "curl/8.10.1", wantName: "curl"},
		{name: "platform only", userAgent: "SomePlayer/1.0 (Linux)", wantName: "Linux device", wantPlatform: "Linux"},
		{name: "unknown", userAgent: "Go-http-client/1.1", wantName: fallbackDeviceName},
		{name: "empty", userAgent: "", wantName: fallbackDeviceName},
		{name: "hostile", userAgent: strings.Repeat("<script>\x00", 500), wantName: fallbackDeviceName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			label := labelUserAgent(test.userAgent)
			if label.name() != test.wantName || label.platform != test.wantPlatform {
				t.Errorf("label = %q on platform %q, want %q on %q", label.name(), label.platform, test.wantName, test.wantPlatform)
			}
		})
	}
}

// TestAuthDeviceLabelsFitTheDeviceLimits: every label the tables can produce is valid for
// auth.Device, and none is taken from the header.
func TestAuthDeviceLabelsFitTheDeviceLimits(t *testing.T) {
	t.Parallel()

	for _, browser := range append(browserMarks(), userAgentMark{}) {
		for _, platform := range append(platformMarks(), userAgentMark{}) {
			label := deviceLabel{browser: browser.label, platform: platform.label}

			if name := label.name(); name == "" || utf8.RuneCountInString(name) > auth.MaxDeviceNameRunes {
				t.Errorf("name %q does not fit %d runes", name, auth.MaxDeviceNameRunes)
			}

			if utf8.RuneCountInString(label.platform) > auth.MaxPlatformRunes {
				t.Errorf("platform %q does not fit %d runes", label.platform, auth.MaxPlatformRunes)
			}
		}
	}
}
