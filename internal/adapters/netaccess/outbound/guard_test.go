package outbound

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestIsPublic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr          string
		allowLoopback bool
		want          bool
	}{
		{"93.184.216.34", false, true},
		{"2606:2800:220:1:248:1893:25c8:1946", false, true},
		{"127.0.0.1", false, false},
		{"127.0.0.1", true, true},
		{"::1", false, false},
		{"10.0.0.1", true, false},
		{"172.16.5.4", false, false},
		{"192.168.1.1", false, false},
		{"169.254.169.254", false, false},
		{"fe80::1", false, false},
		{"fd00:ec2::254", false, false},
		{"fc00::1", false, false},
		{"224.0.0.1", false, false},
		{"ff02::1", false, false},
		{"0.0.0.0", false, false},
		{"100.64.0.1", false, false},
		{"::ffff:10.0.0.1", false, false},
		{"::ffff:127.0.0.1", false, false},
		{"::ffff:93.184.216.34", false, true},
		{"0.1.2.3", false, false},
		{"192.0.0.8", false, false},
		{"192.0.2.10", false, false},
		{"198.18.0.1", false, false},
		{"198.19.255.255", false, false},
		{"198.51.100.7", false, false},
		{"203.0.113.9", false, false},
		{"240.0.0.1", false, false},
		{"255.255.255.255", false, false},
		{"64:ff9b::5db8:d822", false, false},
		{"64:ff9b::a00:1", false, false},
		{"64:ff9b:1::1", false, false},
		{"2002:0a00:0001::1", false, false},
		{"2002:5db8:d822::1", false, false},
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", false, false},
		{"2001:db8::1", false, false},
		{"fec0::1", false, false},
		{"::10.0.0.1", false, false},
		{"::93.184.216.34", false, false},
		{"::", false, false},
		{"::ffff:192.0.2.1", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			t.Parallel()

			if got := isPublic(netip.MustParseAddr(tc.addr), tc.allowLoopback); got != tc.want {
				t.Errorf("isPublic(%s, loopback=%v) = %v, want %v", tc.addr, tc.allowLoopback, got, tc.want)
			}
		})
	}

	if isPublic(netip.Addr{}, true) {
		t.Error("zero address must not be public")
	}
}

type fakeResolver struct {
	answers map[string][]netip.Addr
	err     error
}

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return f.answers[host], f.err
}

func TestGuardedDialerRefusesBeforeDialing(t *testing.T) {
	t.Parallel()

	resolver := fakeResolver{answers: map[string][]netip.Addr{
		"rebind.example": {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.5")},
		"intranet.test":  {netip.MustParseAddr("192.168.0.9")},
	}}
	guard := &guardedDialer{resolver: resolver}

	for _, address := range []string{
		"127.0.0.1:443", "10.0.0.1:443", "169.254.169.254:80", "[::1]:443",
		"[::ffff:10.0.0.1]:443", "intranet.test:443", "rebind.example:443",
	} {
		_, err := guard.dialContext(context.Background(), "tcp", address)
		if !errors.Is(err, ErrBlockedAddress) || !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("dial %s error = %v, want ErrBlockedAddress", address, err)
		}
	}
}

func TestGuardedDialerResolveErrors(t *testing.T) {
	t.Parallel()

	failing := &guardedDialer{resolver: fakeResolver{err: errors.New("nxdomain")}}
	if _, err := failing.dialContext(context.Background(), "tcp", "gone.example:443"); err == nil {
		t.Error("resolver failure must fail the dial")
	}

	empty := &guardedDialer{resolver: fakeResolver{}}
	if _, err := empty.dialContext(context.Background(), "tcp", "empty.example:443"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("empty answer error = %v, want ErrUnavailable", err)
	}

	if _, err := empty.dialContext(context.Background(), "tcp", "no-port"); err == nil {
		t.Error("address without port must fail")
	}
}
