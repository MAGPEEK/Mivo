package clientorigin_test

import (
	"net/netip"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/clientorigin"
)

func TestClientOriginRoundTrip(t *testing.T) {
	t.Parallel()

	want := clientorigin.Origin{
		Address: netip.MustParseAddr("203.0.113.7"), Proxy: netip.MustParseAddr("127.0.0.1"),
		Socket: "127.0.0.1:5000", Secure: true,
	}

	if got := clientorigin.From(clientorigin.With(t.Context(), want)); got != want {
		t.Errorf("From = %+v, want %+v", got, want)
	}
}

func TestLocalClassRoundTripAndMissingIsRemote(t *testing.T) {
	t.Parallel()

	if clientorigin.IsLocal(t.Context()) {
		t.Error("IsLocal without a stored class = true, want the strict reading false")
	}

	if !clientorigin.IsLocal(clientorigin.WithLocal(t.Context(), true)) {
		t.Error("IsLocal after WithLocal(true) = false")
	}

	if clientorigin.IsLocal(clientorigin.WithLocal(t.Context(), false)) {
		t.Error("IsLocal after WithLocal(false) = true")
	}
}

// TestClientOriginMissingIsTheStrictReading: a request that never passed the resolver has no
// address and is not secure.
func TestClientOriginMissingIsTheStrictReading(t *testing.T) {
	t.Parallel()

	got := clientorigin.From(t.Context())
	if got.Address.IsValid() || got.Proxy.IsValid() || got.Secure {
		t.Errorf("From without a stored origin = %+v, want the zero value", got)
	}
}
