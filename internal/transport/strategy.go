// internal/transport/strategy.go
package transport

// TransportStrategy defines which connection types are allowed.
type TransportStrategy interface {
	Name() string
	Description() string
	AllowedTypes() []ConnectionType
}

type simpleStrategy struct {
	name        string
	description string
	types       []ConnectionType
}

func (s simpleStrategy) Name() string {
	return s.name
}

func (s simpleStrategy) Description() string {
	return s.description
}

func (s simpleStrategy) AllowedTypes() []ConnectionType {
	return append([]ConnectionType(nil), s.types...)
}

func defaultStrategies() []TransportStrategy {
	all := allConnectionTypes()
	return []TransportStrategy{
		simpleStrategy{
			name:        "auto",
			description: "All transports with priority-based fallback",
			types:       all,
		},
		simpleStrategy{
			name:        "p2p-only",
			description: "WebRTC only (STUN/TURN)",
			types:       []ConnectionType{WebRTCSTUN, WebRTCTURN},
		},
		simpleStrategy{
			name:        "webrtc-stun",
			description: "Direct WebRTC (STUN only)",
			types:       []ConnectionType{WebRTCSTUN},
		},
		simpleStrategy{
			name:        "relay-safe",
			description: "Relayed or server-assisted (TURN/WebSocket/HTTP)",
			types:       []ConnectionType{WebRTCTURN, WebSocketDirect, HTTPPolling},
		},
		simpleStrategy{
			name:        "websocket-only",
			description: "WebSocket direct only",
			types:       []ConnectionType{WebSocketDirect},
		},
		simpleStrategy{
			name:        "http-only",
			description: "HTTP polling only",
			types:       []ConnectionType{HTTPPolling},
		},
		simpleStrategy{
			name:        "lan-only",
			description: "LAN broadcast only",
			types:       []ConnectionType{LANBroadcast},
		},
	}
}

func allConnectionTypes() []ConnectionType {
	return []ConnectionType{
		WebRTCSTUN,
		WebRTCTURN,
		WebSocketDirect,
		HTTPPolling,
		LANBroadcast,
	}
}
