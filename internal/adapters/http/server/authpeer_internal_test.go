package server

import (
	"net/http"
	"net/netip"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/auth"
)

// TestAuthHandsThePeerToTheHandlers: sign-in, setup and the session routes get the classified
// peer, which the sign-in throttle counts by and the device record stores.
func TestAuthHandsThePeerToTheHandlers(t *testing.T) {
	t.Parallel()

	const (
		lanAddress    = "192.168.1.20"
		remoteAddress = "8.8.8.8"
	)

	tests := []authCase{
		{name: "sign-in from the local network", method: http.MethodPost, target: apiV1AuthSession, body: "{}", remoteAddr: lanAddress + ":5000"},
		{name: "sign-in from outside", method: http.MethodPost, target: apiV1AuthSession, body: "{}", remoteAddr: remoteAddress + ":5000"},
		{name: "setup", method: http.MethodPost, target: apiV1Setup, body: "{}", remoteAddr: lanAddress + ":5000", setup: fakeSetup{required: true}},
		{name: "password change", method: http.MethodPut, target: apiV1Me + "/password", body: "{}", remoteAddr: remoteAddress + ":5000", bearer: userToken},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var got auth.Peer

			test.allowPublic = true
			options := authOptions(test, &fakeResolver{})
			options.APIHandler = http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				got = restapi.PeerFrom(request.Context())
			})

			recorder := serveChain(t, options, newAuthRequest(t, test))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d %s, want 200", recorder.Code, recorder.Body)
			}

			address, _ := netip.ParseAddrPort(test.remoteAddr)
			want := auth.Peer{Address: address.Addr(), Class: auth.AddressRemote}

			if address.Addr().String() == lanAddress {
				want.Class = auth.AddressLAN
			}

			if got != want {
				t.Errorf("peer = %+v, want %+v", got, want)
			}
		})
	}
}
