package config_test

import (
	"errors"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/platform/config"
)

func prefixes(texts ...string) []netip.Prefix {
	parsed := make([]netip.Prefix, 0, len(texts))
	for _, text := range texts {
		parsed = append(parsed, netip.MustParsePrefix(text))
	}

	return parsed
}

func TestLoadTrustedProxies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		yaml string
		vars map[string]string
		want []netip.Prefix
	}{
		{name: "default trusts nobody"},
		{
			name: "file with prefixes and single addresses",
			yaml: "server:\n  trusted_proxies:\n    - 10.0.0.0/8\n    - 192.168.1.2\n    - 'fd00::/8'\n    - '::1'\n",
			want: prefixes("10.0.0.0/8", "192.168.1.2/32", "fd00::/8", "::1/128"),
		},
		{name: "empty file list", yaml: "server:\n  trusted_proxies: []\n"},
		{
			name: "entries are stored masked",
			yaml: "server:\n  trusted_proxies: ['192.168.1.77/24', '2001:db8::1/32']\n",
			want: prefixes("192.168.1.0/24", "2001:db8::/32"),
		},
		{
			name: "mapped entries are unmapped",
			yaml: "server:\n  trusted_proxies: ['::ffff:192.168.1.2', '::ffff:10.1.2.3/104']\n",
			want: prefixes("192.168.1.2/32", "10.0.0.0/8"),
		},
		{
			name: "environment list with spaces",
			vars: map[string]string{env("SERVER_TRUSTED_PROXIES"): " 127.0.0.1 , ::1,172.16.0.0/12 "},
			want: prefixes("127.0.0.1/32", "::1/128", "172.16.0.0/12"),
		},
		{
			name: "environment replaces the file", yaml: "server:\n  trusted_proxies: [10.0.0.1]\n",
			vars: map[string]string{env("SERVER_TRUSTED_PROXIES"): "10.0.0.2"}, want: prefixes("10.0.0.2/32"),
		},
		{
			name: "empty environment trusts nobody", yaml: "server:\n  trusted_proxies: [10.0.0.1]\n",
			vars: map[string]string{env("SERVER_TRUSTED_PROXIES"): ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := newFixture(t, tt.yaml, tt.vars).load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if !slices.Equal(got.Server.TrustedProxies, tt.want) {
				t.Errorf("Server.TrustedProxies = %v, want %v", got.Server.TrustedProxies, tt.want)
			}
		})
	}
}

func TestLoadTrustedProxiesInvalid(t *testing.T) {
	t.Parallel()

	variable := env("SERVER_TRUSTED_PROXIES")

	tests := []struct {
		name string
		yaml string
		vars map[string]string
		// wantEntry is the offending entry the error must quote.
		wantEntry string
	}{
		{name: "not an address", yaml: "server:\n  trusted_proxies: [proxy.local]\n", wantEntry: "proxy.local"},
		{name: "bad prefix length", yaml: "server:\n  trusted_proxies: [10.0.0.0/33]\n", wantEntry: "10.0.0.0/33"},
		{name: "address with port", yaml: "server:\n  trusted_proxies: ['10.0.0.1:80']\n", wantEntry: "10.0.0.1:80"},
		{name: "zone", yaml: "server:\n  trusted_proxies: ['fe80::1%eth0']\n", wantEntry: "fe80::1%eth0"},
		{name: "everything ipv4", yaml: "server:\n  trusted_proxies: [0.0.0.0/0]\n", wantEntry: "0.0.0.0/0"},
		{name: "everything ipv6", yaml: "server:\n  trusted_proxies: ['::/0']\n", wantEntry: "::/0"},
		{name: "any length 0", yaml: "server:\n  trusted_proxies: [10.0.0.0/0]\n", wantEntry: "10.0.0.0/0"},
		{name: "mapped everything", yaml: "server:\n  trusted_proxies: ['::ffff:0.0.0.0/96']\n", wantEntry: "::ffff:0.0.0.0/96"},
		{name: "mapped prefix shorter than the mapping", yaml: "server:\n  trusted_proxies: ['::ffff:10.0.0.0/64']\n", wantEntry: "::ffff:10.0.0.0/64"},
		{name: "duplicate", yaml: "server:\n  trusted_proxies: [10.0.0.1, 10.0.0.1]\n", wantEntry: "10.0.0.1"},
		{name: "duplicate after masking", yaml: "server:\n  trusted_proxies: [10.0.0.0/8, 10.9.9.9/8]\n", wantEntry: "10.9.9.9/8"},
		{name: "duplicate after unmapping", yaml: "server:\n  trusted_proxies: [10.0.0.1, '::ffff:10.0.0.1']\n", wantEntry: "::ffff:10.0.0.1"},
		{name: "empty file entry", yaml: "server:\n  trusted_proxies: ['']\n", wantEntry: `""`},
		{name: "environment garbage", vars: map[string]string{variable: "10.0.0.1,nonsense"}, wantEntry: "nonsense"},
		{name: "environment empty entry", vars: map[string]string{variable: "10.0.0.1,,10.0.0.2"}, wantEntry: `""`},
		{name: "environment everything", vars: map[string]string{variable: "0.0.0.0/0"}, wantEntry: "0.0.0.0/0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := newFixture(t, tt.yaml, tt.vars).load()
			if !errors.Is(err, config.ErrInvalid) {
				t.Fatalf("Load error = %v, want it to wrap ErrInvalid", err)
			}

			for _, want := range []string{"server.trusted_proxies", tt.wantEntry} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Load error = %v, want it to contain %q", err, want)
				}
			}

			if len(tt.vars) > 0 && !strings.Contains(err.Error(), variable) {
				t.Errorf("Load error = %v, want it to name %s", err, variable)
			}
		})
	}
}

func TestLoadTrustedProxiesRejectsOtherShapes(t *testing.T) {
	t.Parallel()

	for _, yaml := range []string{
		"server:\n  trusted_proxies: 10.0.0.1\n",
		"server:\n  trusted_proxies:\n",
		"server:\n  trusted_proxies: [[10.0.0.1]]\n",
	} {
		_, err := newFixture(t, yaml, nil).load()
		if !errors.Is(err, config.ErrInvalid) || !strings.Contains(err.Error(), "server.trusted_proxies") {
			t.Errorf("Load(%q) error = %v, want ErrInvalid naming the key", yaml, err)
		}
	}
}
