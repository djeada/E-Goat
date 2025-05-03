package webrtc

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/pion/webrtc/v3"
)

// Message represents a signaling message exchanged via signaling server.
type Message struct {
	PeerID  string `json:"peer_id"`
	Type    string `json:"type"`    // "offer", "answer", "ice"
	Payload string `json:"payload"` // SDP or ICE JSON
}

// Peer wraps a Pion PeerConnection and a data channel.
type Peer struct {
	PeerID    string
	Conn      *webrtc.PeerConnection
	Channel   *webrtc.DataChannel
	oncClose  func()
	onceClose sync.Once
}

// Config for creating new Peers
var defaultConfig = webrtc.Configuration{
	ICEServers: []webrtc.ICEServer{
		{URLs: []string{"stun:stun.l.google.com:19302"}},
	},
}

// NewPeer initializes a PeerConnection and sets up handlers.
func NewPeer(peerID string, onMessage func(from string, data []byte), sendSignal func(msg Message) error, onClose func()) (*Peer, error) {
	// Create PeerConnection
	pc, err := webrtc.NewPeerConnection(defaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create PeerConnection: %w", err)
	}

	p := &Peer{PeerID: peerID, Conn: pc, oncClose: onClose}

	// Handle ICE candidates
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		cand, err := json.Marshal(c.ToJSON())
		if err != nil {
			log.Printf("ICE marshal error: %v", err)
			return
		}
		signal := Message{PeerID: peerID, Type: "ice", Payload: string(cand)}
		if err := sendSignal(signal); err != nil {
			log.Printf("failed to send ICE candidate: %v", err)
		}
	})

	// DataChannel for chat
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		p.Channel = dc
		dc.OnOpen(func() {
			log.Printf("DataChannel '%s'-'%d' open\n", dc.Label(), dc.ID())
		})
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			onMessage(peerID, msg.Data)
		})
	})

	// If initiating, create a data channel
	dc, err := pc.CreateDataChannel("chat", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create DataChannel: %w", err)
	}
	p.Channel = dc
	dc.OnOpen(func() {
		log.Printf("DataChannel '%s'-'%d' open\n", dc.Label(), dc.ID())
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		onMessage(peerID, msg.Data)
	})

	// gather local description and send offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return nil, fmt.Errorf("CreateOffer error: %w", err)
	}
	if err = pc.SetLocalDescription(offer); err != nil {
		return nil, fmt.Errorf("SetLocalDescription error: %w", err)
	}
	// wait for ICE gathering
	// use OnICEGatheringStateChange
	pc.OnICEGatheringStateChange(func(state webrtc.ICEGathererState) {
		if state == webrtc.ICEGathererStateComplete {
			sdp := pc.LocalDescription().SDP
			signal := Message{PeerID: peerID, Type: "offer", Payload: sdp}
			if err := sendSignal(signal); err != nil {
				log.Printf("failed to send offer: %v", err)
			}
		}
	})

	// Handle connection state
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("Peer %s state: %s", peerID, state.String())
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateDisconnected {
			p.close()
		}
	})

	return p, nil
}

// HandleSignal processes incoming signaling messages.
func (p *Peer) HandleSignal(msg Message) error {
	switch msg.Type {
	case "offer":
		desc := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: msg.Payload}
		if err := p.Conn.SetRemoteDescription(desc); err != nil {
			return err
		}
		answer, err := p.Conn.CreateAnswer(nil)
		if err != nil {
			return err
		}
		if err := p.Conn.SetLocalDescription(answer); err != nil {
			return err
		}
		// send answer back
		signal := Message{PeerID: p.PeerID, Type: "answer", Payload: p.Conn.LocalDescription().SDP}
		return p.sendSignal(signal)

	case "answer":
		desc := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: msg.Payload}
		return p.Conn.SetRemoteDescription(desc)

	case "ice":
		var cand webrtc.ICECandidateInit
		if err := json.Unmarshal([]byte(msg.Payload), &cand); err != nil {
			return err
		}
		return p.Conn.AddICECandidate(cand)

	default:
		return fmt.Errorf("unknown signal type: %s", msg.Type)
	}
}

// SendMessage sends data to the remote via DataChannel
func (p *Peer) SendMessage(data []byte) error {
	if p.Channel == nil {
		return fmt.Errorf("data channel not ready")
	}
	return p.Channel.Send(data)
}

func (p *Peer) close() {
	p.onceClose.Do(func() {
		if p.oncClose != nil {
			p.oncClose()
		}
		p.Conn.Close()
	})
}
