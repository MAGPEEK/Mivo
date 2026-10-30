package archtest

import (
	"slices"
	"testing"
)

func TestCheckSourceAppliesLayerMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		file          string
		source        string
		wantForbidden []string
	}{
		{
			name: "domain may import std lib and itself",
			file: "internal/domain/media/item.go",
			source: `package media
import (
	"errors"
	"net/url"
	"net/netip"
	"net/mail"
	"io"
	"crypto/sha256"
	"github.com/magpeek/mivo/internal/domain/library"
)`,
		},
		{
			name: "domain must not do I/O or import outer layers and third party",
			file: "internal/domain/media/item.go",
			source: `package media
import (
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"net"
	"net/http/httptest"
	"net/rpc"
	"net/rpc/jsonrpc"
	"net/smtp"
	"net/textproto"
	"database/sql/driver"
	"syscall"
	"log/syslog"
	"plugin"
	"io/ioutil"
	"expvar"
	"crypto/tls"
	"C"
	"github.com/magpeek/mivo/internal/app"
	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/cmd/tool"
	"example.com/thirdparty"
)`,
			wantForbidden: []string{
				"os", "os/exec", "os/signal", "os/user", "net", "net/http/httptest",
				"net/rpc", "net/rpc/jsonrpc", "net/smtp", "net/textproto", "database/sql/driver", "syscall",
				"log/syslog", "plugin", "io/ioutil", "expvar", "crypto/tls", "C",
				"github.com/magpeek/mivo/internal/app", "github.com/magpeek/mivo/internal/platform/brand",
				"github.com/magpeek/mivo/internal/adapters/sqlite", "github.com/magpeek/mivo/cmd/tool",
				"example.com/thirdparty",
			},
		},
		{
			name: "app may import std lib, domain and app",
			file: "internal/app/playback/start.go",
			source: `package playback
import (
	"context"
	"os"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/app/library"
)`,
		},
		{
			name: "app must not import adapters, platform, processes, HTTP or SQL",
			file: "internal/app/playback/start.go",
			source: `package playback
import (
	"os/exec"
	"net/http"
	"database/sql"
	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/cmd/tool"
	"example.com/thirdparty"
)`,
			wantForbidden: []string{
				"os/exec", "net/http", "database/sql",
				"github.com/magpeek/mivo/internal/adapters/ffmpeg",
				"github.com/magpeek/mivo/internal/platform/config",
				"github.com/magpeek/mivo/cmd/tool", "example.com/thirdparty",
			},
		},
		{
			name: "platform may import std lib, third party and platform",
			file: "internal/platform/config/load.go",
			source: `package config
import (
	"os"
	"example.com/thirdparty"
	"github.com/magpeek/mivo/internal/platform/brand"
)`,
		},
		{
			name: "platform must not import domain, app, adapters or cmd",
			file: "internal/platform/config/load.go",
			source: `package config
import (
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/app/system"
	"github.com/magpeek/mivo/internal/adapters/http"
	"github.com/magpeek/mivo/cmd/tool"
)`,
			wantForbidden: []string{
				"github.com/magpeek/mivo/internal/domain", "github.com/magpeek/mivo/internal/app/system",
				"github.com/magpeek/mivo/internal/adapters/http", "github.com/magpeek/mivo/cmd/tool",
			},
		},
		{
			name: "adapters may import every layer",
			file: "internal/adapters/sqlite/store.go",
			source: `package sqlite
import (
	"database/sql"
	"example.com/thirdparty"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/app"
	"github.com/magpeek/mivo/internal/platform/config"
)`,
		},
		{
			name: "adapters must not import cmd",
			file: "internal/adapters/sqlite/store.go",
			source: `package sqlite
import "github.com/magpeek/mivo/cmd/tool"`,
			wantForbidden: []string{"github.com/magpeek/mivo/cmd/tool"},
		},
		{
			name: "files outside the layers are unrestricted",
			file: "internal/archtest/layers.go",
			source: `package archtest
import (
	"os"
	"github.com/magpeek/mivo/internal/adapters/http"
)`,
		},
		{
			name: "a directory sharing a layer prefix is not that layer",
			file: "internal/domainx/thing.go",
			source: `package domainx
import "os"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			violations, err := checkSource(test.file, []byte(test.source), layerMatrix())
			if err != nil {
				t.Fatalf("checkSource: %v", err)
			}

			if got := forbiddenImports(violations); !slices.Equal(got, test.wantForbidden) {
				t.Errorf("forbidden imports = %q, want %q", got, test.wantForbidden)
			}
		})
	}
}

func TestCheckSourceRejectsUnparsableFile(t *testing.T) {
	t.Parallel()

	if _, err := checkSource("internal/domain/broken.go", []byte("not go"), layerMatrix()); err == nil {
		t.Fatal("checkSource accepted a file without package clause")
	}
}

func TestViolationStringNamesFileImportAndReason(t *testing.T) {
	t.Parallel()

	violation := Violation{File: "internal/domain/a.go", Import: "os", Reason: "internal/domain must not import it"}

	want := `internal/domain/a.go imports "os": internal/domain must not import it`
	if got := violation.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestImportRuleMatchesExactOrPathPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rule       importRule
		importPath string
		want       bool
	}{
		{rule: prefix("net/http"), importPath: "net/http", want: true},
		{rule: prefix("net/http"), importPath: "net/http/httptest", want: true},
		{rule: prefix("net/http"), importPath: "net/httpx", want: false},
		{rule: exactly("net"), importPath: "net", want: true},
		{rule: exactly("net"), importPath: "net/url", want: false},
		{rule: prefix("os"), importPath: "os/signal", want: true},
		{rule: prefix("os"), importPath: "osx", want: false},
	}

	for _, test := range tests {
		if got := test.rule.matches(test.importPath); got != test.want {
			t.Errorf("%+v.matches(%q) = %t, want %t", test.rule, test.importPath, got, test.want)
		}
	}
}

func forbiddenImports(violations []Violation) []string {
	var imports []string
	for _, violation := range violations {
		imports = append(imports, violation.Import)
	}

	return imports
}
