package spkmeta

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// DSM main menu entry of the package (DSM 7.2.2 Package Developer Guide, "Synology DSM
// Integration" - "Desktop Application" - "Application Config", and INFO "dsmuidir"): INFO
// dsmuidir names a folder of package.tgz that DSM links to
// /usr/syno/synoman/webman/3rdparty/<package>; its file "config" declares the menu entries,
// and the icons lie next to it.
const (
	// DesktopUIDir is the dsmuidir folder in package.tgz. INFO.sh and SynoBuildConf/install
	// spell the same name.
	DesktopUIDir = "ui"
	// DesktopConfigFile is the name of the application config in DesktopUIDir.
	DesktopConfigFile = "config"
	// desktopImagesDir holds the icons, relative to DesktopUIDir.
	desktopImagesDir = "images"
	// desktopSizePlaceholder is the guide's template field DSM replaces with an icon size.
	desktopSizePlaceholder = "{0}"
	// desktopAppType opens the url in a new browser window ("url" is the one documented type).
	desktopAppType = "url"
	// desktopProtocol is the scheme of the web app; the server speaks plain HTTP on the LAN.
	desktopProtocol = "http"
	// desktopPath is the web app's path on the server.
	desktopPath = "/"
	// desktopGrantAll lets an administrator grant the entry to every user and group in
	// Control Panel > Application Privileges (community practice, not in the guide).
	desktopGrantAll = "all"
	// desktopDescription is the mouse-over text of the menu entry.
	desktopDescription = "Opens the web app of the media server"
)

// desktopApp is one entry of the ".url" object of the application config. The guide documents
// type, icon, title, desc, url and allUsers; protocol and port (the url is then a path on the
// NAS at that port) and the two privilege keys are what community packages such as spksrc
// and Plex write for DSM 7 and are unconfirmed (DSM-NOTES section 16).
type desktopApp struct {
	Type                  string `json:"type"`
	Title                 string `json:"title"`
	Description           string `json:"desc"`
	Icon                  string `json:"icon"`
	Protocol              string `json:"protocol"`
	Port                  string `json:"port"`
	URL                   string `json:"url"`
	AllUsers              bool   `json:"allUsers"`
	GrantPrivilege        string `json:"grantPrivilege"`
	AdvanceGrantPrivilege bool   `json:"advanceGrantPrivilege"`
}

// DesktopIconSizes returns the pixel sizes of the main menu icons, smallest first: the sizes
// the guide names for the {0} of "icon".
func DesktopIconSizes() []int {
	return []int{16, 24, 32, 48, 64, 72, 256}
}

// DesktopIconPath returns the path of the main menu icon of the given size relative to
// DesktopUIDir: images/<id>_<size>.png.
func (pkg Package) DesktopIconPath(size int) string {
	return strings.Replace(pkg.desktopIconTemplate(), desktopSizePlaceholder, strconv.Itoa(size), 1)
}

// desktopIconTemplate is the "icon" value: images/<id>_{0}.png.
func (pkg Package) desktopIconTemplate() string {
	return desktopImagesDir + "/" + pkg.ID + "_" + desktopSizePlaceholder + ".png"
}

// desktopAppKey is the unique property name of the entry: a reverse domain of the maintainer
// (desktopMaintainerLabel) plus the package name.
func (pkg Package) desktopAppKey() string {
	return "com." + pkg.desktopMaintainerLabel() + "." + pkg.ID
}

// desktopMaintainerLabel is the maintainer's ASCII letters and digits in lower case, the
// maintainer's part of desktopAppKey. Validate refuses a maintainer for which it is empty.
func (pkg Package) desktopMaintainerLabel() string {
	return strings.Map(func(character rune) rune {
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9':
			return character
		case character >= 'A' && character <= 'Z':
			return character - 'A' + 'a'
		default:
			return -1
		}
	}, pkg.Maintainer)
}

// validateDesktopAppKey refuses a maintainer without ASCII letters or digits, which would
// leave an empty label in the entry key ("com..<id>").
func (pkg Package) validateDesktopAppKey() error {
	if pkg.desktopMaintainerLabel() == "" {
		return fmt.Errorf("MAINTAINER %q has no ASCII letter or digit for the %s entry key: %w",
			pkg.Maintainer, DesktopConfigFile, ErrInvalidValue)
	}

	return nil
}

// DesktopAppConfig renders the application config: one "url" entry titled with the display
// name that opens http://<NAS>:<port>/ for every user, with a final newline.
func (pkg Package) DesktopAppConfig() ([]byte, error) {
	if err := pkg.Validate(); err != nil {
		return nil, fmt.Errorf("render %s: %w", DesktopConfigFile, err)
	}

	config := map[string]map[string]desktopApp{
		".url": {
			pkg.desktopAppKey(): {
				Type:                  desktopAppType,
				Title:                 pkg.DisplayName,
				Description:           desktopDescription,
				Icon:                  pkg.desktopIconTemplate(),
				Protocol:              desktopProtocol,
				Port:                  strconv.Itoa(pkg.Port),
				URL:                   desktopPath,
				AllUsers:              true,
				GrantPrivilege:        desktopGrantAll,
				AdvanceGrantPrivilege: true,
			},
		},
	}

	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", DesktopConfigFile, err)
	}

	return append(content, '\n'), nil
}
