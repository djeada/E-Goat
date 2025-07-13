// webrtc_test.js - Test WebRTC functionality
const WebSocket = require('ws');

console.log('🧪 Starting WebRTC signaling test...');

// Create two mock peers
const peer1 = {
    id: 'test-peer-1',
    ws: null
};

const peer2 = {
    id: 'test-peer-2', 
    ws: null
};

const room = 'test-room';

// Connect peer 1
peer1.ws = new WebSocket(`ws://localhost:9000/signal?room=${room}&peer_id=${peer1.id}`);

peer1.ws.on('open', () => {
    console.log('✅ Peer 1 connected to signaling server');
    
    // Connect peer 2 after peer 1 is connected
    setTimeout(() => {
        peer2.ws = new WebSocket(`ws://localhost:9000/signal?room=${room}&peer_id=${peer2.id}`);
        
        peer2.ws.on('open', () => {
            console.log('✅ Peer 2 connected to signaling server');
        });
        
        peer2.ws.on('message', (data) => {
            const msg = JSON.parse(data);
            console.log('📨 Peer 2 received:', msg);
            
            if (msg.type === 'peer_joined') {
                console.log('👋 Peer 2 sees that peer 1 joined');
            }
        });
        
        peer2.ws.on('error', (error) => {
            console.log('❌ Peer 2 WebSocket error:', error.message);
        });
        
    }, 1000);
});

peer1.ws.on('message', (data) => {
    const msg = JSON.parse(data);
    console.log('📨 Peer 1 received:', msg);
    
    if (msg.type === 'peer_joined') {
        console.log('👋 Peer 1 sees that peer 2 joined');
        
        // Simulate sending an offer
        setTimeout(() => {
            console.log('📤 Peer 1 sending mock offer to peer 2');
            const offer = {
                peer_id: peer1.id,
                target_peer_id: peer2.id,
                type: 'offer',
                payload: JSON.stringify({
                    type: 'offer',
                    sdp: 'mock-offer-sdp-data'
                })
            };
            peer1.ws.send(JSON.stringify(offer));
        }, 500);
    }
});

peer1.ws.on('error', (error) => {
    console.log('❌ Peer 1 WebSocket error:', error.message);
});

// Cleanup after 10 seconds
setTimeout(() => {
    console.log('🧹 Cleaning up test connections...');
    if (peer1.ws) peer1.ws.close();
    if (peer2.ws) peer2.ws.close();
    process.exit(0);
}, 10000);
