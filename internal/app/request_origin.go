package app

import (
	"net/http"
	"net/netip"
	"strings"
)

func peerAddress(r *http.Request) netip.Addr {
	if peer, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return peer.Addr().WithZone("").Unmap()
	}
	peer, _ := netip.ParseAddr(r.RemoteAddr)
	return peer.WithZone("").Unmap()
}

func (a *App) trustsProxy(peer netip.Addr) bool {
	for _, prefix := range a.trustedProxies {
		if prefix.Contains(peer) {
			return true
		}
	}
	return false
}

func (a *App) requestOrigin(r *http.Request) string {
	peer := peerAddress(r)
	if !a.trustsProxy(peer) {
		return peer.String()
	}
	// Read the chain from the known peer towards the client. The first
	// untrusted hop is the boundary beyond which the client can forge entries.
	values := r.Header.Values("X-Forwarded-For")
	for i := len(values) - 1; i >= 0; i-- {
		addresses := strings.Split(values[i], ",")
		for j := len(addresses) - 1; j >= 0; j-- {
			address, err := netip.ParseAddr(strings.TrimSpace(addresses[j]))
			if err != nil || address.Zone() != "" {
				return peer.String()
			}
			address = address.Unmap()
			if !a.trustsProxy(address) {
				return address.String()
			}
		}
	}
	return peer.String()
}

func (a *App) secureSession(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// A configured proxy must overwrite this header with the external scheme.
	protos := r.Header.Values("X-Forwarded-Proto")
	return a.trustsProxy(peerAddress(r)) && len(protos) == 1 && strings.EqualFold(strings.TrimSpace(protos[0]), "https")
}
