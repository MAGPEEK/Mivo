package restapi

import "strings"

// fallbackDeviceName names a device whose User-Agent says nothing we recognise.
const fallbackDeviceName = "Unknown device"

// userAgentMark maps a fragment of a User-Agent to the fixed label shown for it.
type userAgentMark struct {
	fragment string
	label    string
}

// browserMarks are checked in order. Order matters: Edge, Opera and Samsung Internet also
// name Chrome, and every Chromium browser also names Safari.
func browserMarks() []userAgentMark {
	return []userAgentMark{
		{fragment: "Edg/", label: "Edge"},
		{fragment: "EdgA/", label: "Edge"},
		{fragment: "EdgiOS/", label: "Edge"},
		{fragment: "OPR/", label: "Opera"},
		{fragment: "SamsungBrowser/", label: "Samsung Internet"},
		{fragment: "Firefox/", label: "Firefox"},
		{fragment: "FxiOS/", label: "Firefox"},
		{fragment: "Chrome/", label: "Chrome"},
		{fragment: "CriOS/", label: "Chrome"},
		{fragment: "Safari/", label: "Safari"},
		{fragment: "curl/", label: "curl"},
	}
}

// platformMarks are checked in order. Order matters: webOS, Tizen, Android and ChromeOS also
// name Linux, and iOS devices also name "Mac OS X".
func platformMarks() []userAgentMark {
	return []userAgentMark{
		{fragment: "Web0S", label: "webOS"},
		{fragment: "webOS", label: "webOS"},
		{fragment: "Tizen", label: "Tizen"},
		{fragment: "Android", label: "Android"},
		{fragment: "iPhone", label: "iOS"},
		{fragment: "iPad", label: "iOS"},
		{fragment: "iPod", label: "iOS"},
		{fragment: "Windows", label: "Windows"},
		{fragment: "CrOS", label: "ChromeOS"},
		{fragment: "Macintosh", label: "macOS"},
		{fragment: "Mac OS X", label: "macOS"},
		{fragment: "Linux", label: "Linux"},
	}
}

// deviceLabel is what a User-Agent tells about a device. It holds only labels from the fixed
// tables above, never text of the header, so the raw User-Agent is not stored and both
// labels stay far below auth.MaxDeviceNameRunes and auth.MaxPlatformRunes.
type deviceLabel struct {
	browser  string
	platform string
}

// labelUserAgent recognises the browser and the platform of a User-Agent header.
func labelUserAgent(userAgent string) deviceLabel {
	return deviceLabel{
		browser:  firstMark(userAgent, browserMarks()),
		platform: firstMark(userAgent, platformMarks()),
	}
}

func firstMark(userAgent string, marks []userAgentMark) string {
	for _, mark := range marks {
		if strings.Contains(userAgent, mark.fragment) {
			return mark.label
		}
	}

	return ""
}

// name is the default device name, for example "Firefox on Windows".
func (label deviceLabel) name() string {
	switch {
	case label.browser != "" && label.platform != "":
		return label.browser + " on " + label.platform
	case label.browser != "":
		return label.browser
	case label.platform != "":
		return label.platform + " device"
	default:
		return fallbackDeviceName
	}
}
