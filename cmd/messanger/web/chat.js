// web/chat.js

// We support both HTTP polling and WebRTC connections
const httpOrigin = location.origin;
const wsOrigin = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.hostname}:9000`;

let room, peerId;
let pollInterval;
let lastTs = 0;  // UNIX timestamp of the last‐seen message

// WebRTC and Transport variables
let signalingWS = null;
let useTransportLayer = true;
let connectedPeers = new Set();
let transportStatus = 'disconnected';
let peerConnections = new Map(); // Store WebRTC peer connections
let dataChannels = new Map(); // Store WebRTC data channels

// WebRTC configuration with multiple STUN servers for better connectivity
const rtcConfig = {
  iceServers: [
    { urls: 'stun:stun.l.google.com:19302' },
    { urls: 'stun:stun1.l.google.com:19302' },
    { urls: 'stun:stun2.l.google.com:19302' },
    { urls: 'stun:stun.cloudflare.com:3478' },
    { urls: 'stun:stun.nextcloud.com:443' }
  ],
  iceCandidatePoolSize: 10,
  bundlePolicy: 'balanced'
};

window.addEventListener("load", () => {
  // Generate a unique ID for ourselves
  peerId = crypto.randomUUID();

  // Fetch our external IP (for the invite link)
  fetchIP();

  // Auto‐join if deep‐linked
  const params    = new URLSearchParams(location.search);
  const urlRoom   = params.get("room");
  const urlPeerId = params.get("peer_id");
  if (urlRoom && urlPeerId) {
    peerId = urlPeerId;
    document.getElementById("room-input").value = urlRoom;
    joinRoom();
  }

  // Always bind buttons
  document.getElementById("create-room-btn")
          .addEventListener("click", joinRoom);
  document.getElementById("send-btn")
          .addEventListener("click", sendMessage);
          
  // Add transport controls
  document.getElementById("transport-toggle")
          .addEventListener("click", toggleTransport);
  document.getElementById("connect-peer-btn")
          .addEventListener("click", connectToPeer);
});

// 1 Get your external IP for display
async function fetchIP() {
  try {
    const res = await fetch("https://api.ipify.org?format=json");
    const { ip } = await res.json();
    myIP = ip;
    document.getElementById("my-ip").textContent = ip;
  } catch {
    document.getElementById("my-ip").textContent = "Unknown";
  }
}

// 2 Create/join a room
function joinRoom() {
  room = document.getElementById("room-input").value.trim();
  if (!room) {
    return alert("Please enter a room name.");
  }

  // Build and show the HTTP deep‐link invite
  const invite = `${httpOrigin}/?room=${room}&peer_id=${peerId}`;
  document.getElementById("invite-text"     ).value = invite;
  document.getElementById("invite-text-chat").value = invite;
  document.getElementById("invitation").classList.remove("hidden");

  // Swap to chat UI
  document.getElementById("init").classList.add("hidden");
  document.getElementById("chat").classList.remove("hidden");
  document.getElementById("room-info").textContent = `Room: ${room}`;

  // Start polling for new messages once per second
  pollHistory();
  pollInterval = setInterval(pollHistory, 1000);
  
  // Initialize transport layer if enabled
  if (useTransportLayer) {
    initializeTransport();
  }
  
  // Update transport status display
  updateTransportStatus();
}

// 3 Poll the server for new messages since lastTs
async function pollHistory() {
  try {
    const res = await fetch(
      `${httpOrigin}/history?room=${encodeURIComponent(room)}&since=${lastTs}`
    );
    if (!res.ok) throw new Error(res.statusText);
    const messages = await res.json();
    for (const msg of messages) {
      appendMessage(msg.peer_id, msg.text);
      if (msg.timestamp > lastTs) lastTs = msg.timestamp;
    }
  } catch (e) {
    console.error("History poll failed:", e);
  }
}

// 4 Send a new chat message via POST /send or transport layer
async function sendMessage() {
  const input = document.getElementById("msg-input");
  const text  = input.value.trim();
  if (!text) return;

  // Optimistic UI
  appendMessage("Me", text);
  input.value = "";

  // Try WebRTC data channels first if available
  if (useTransportLayer && dataChannels.size > 0) {
    try {
      const message = JSON.stringify({ from: peerId, text: text });
      let sent = false;
      
      console.log(`🚀 Attempting WebRTC send to ${dataChannels.size} channel(s)`);
      
      for (const [peerID, channel] of dataChannels) {
        console.log(`📡 Channel ${peerID} state: ${channel.readyState}`);
        if (channel.readyState === 'open') {
          channel.send(message);
          sent = true;
          console.log(`✅ Message sent via WebRTC to ${peerID}`);
        } else {
          console.log(`⚠️  Channel ${peerID} not open (${channel.readyState})`);
        }
      }
      
      if (sent) {
        appendMessage("System", `🚀 ✅ Sent via WebRTC to ${dataChannels.size} peer(s)`);
        return;
      } else {
        throw new Error("No open data channels");
      }
    } catch (e) {
      console.error("❌ WebRTC send failed, trying transport layer:", e);
      appendMessage("System", `❌ WebRTC send failed: ${e.message}`);
      
      // Try transport layer REST API
      if (connectedPeers.size > 0) {
        try {
          for (const peerID of connectedPeers) {
            await sendViaTransport(peerID, text);
          }
          appendMessage("System", `📡 Sent via transport layer to ${connectedPeers.size} peer(s)`);
          return;
        } catch (e2) {
          console.error("❌ Transport send failed, falling back to HTTP:", e2);
          appendMessage("System", "🔄 Transport failed, falling back to HTTP polling");
        }
      }
    }
  } else {
    console.log(`⚠️  WebRTC not available: useTransportLayer=${useTransportLayer}, dataChannels=${dataChannels.size}`);
    appendMessage("System", `⚠️  No WebRTC channels available, using fallback`);
  }

  // Fallback to HTTP polling
  try {
    const res = await fetch(`${httpOrigin}/send`, {
      method:  "POST",
      headers: { "Content-Type": "application/json" },
      body:    JSON.stringify({ room, peer_id: peerId, text }),
    });
    if (!res.ok) throw new Error(res.statusText);
    const { timestamp } = await res.json();
    if (timestamp > lastTs) lastTs = timestamp;
  } catch (e) {
    // Show error in chat
    appendMessage("Error", `Failed to send: ${e.message}`);
    console.error("Send failed:", e);
  }
}

// 5 Helper to append a line to the chat box
function appendMessage(from, txt) {
  const container = document.getElementById("messages");
  const line      = document.createElement("div");
  line.textContent = `${from}: ${txt}`;
  container.appendChild(line);
  container.scrollTop = container.scrollHeight;
}

// === TRANSPORT LAYER FUNCTIONS ===

// Initialize transport layer with WebRTC signaling
async function initializeTransport() {
  try {
    // Connect to signaling WebSocket
    const wsUrl = `${wsOrigin}/signal?room=${encodeURIComponent(room)}&peer_id=${encodeURIComponent(peerId)}`;
    signalingWS = new WebSocket(wsUrl);
    
    signalingWS.onopen = () => {
      appendMessage("System", "🔌 Connected to signaling server");
      transportStatus = 'signaling_connected';
      updateTransportStatus();
    };
    
    signalingWS.onmessage = (event) => {
      try {
        const signal = JSON.parse(event.data);
        console.log(`📡 Received signal from ${signal.peer_id}: ${signal.type}`);
        
        if (signal.type === 'peer_joined') {
          // Auto-connect to new peers that join the room
          appendMessage("System", `👋 ${signal.peer_id} joined the room`);
          setTimeout(() => autoConnectToPeer(signal.peer_id), 1000);
        } else {
          appendMessage("System", `📡 Signal from ${signal.peer_id}: ${signal.type}`);
          handleSignalingMessage(signal);
        }
      } catch (e) {
        console.error("Signaling message error:", e);
      }
    };
    
    signalingWS.onclose = () => {
      appendMessage("System", "🔌 Signaling connection closed");
      transportStatus = 'disconnected';
      updateTransportStatus();
    };
    
    signalingWS.onerror = (error) => {
      appendMessage("System", `❌ Signaling error: ${error}`);
      transportStatus = 'error';
      updateTransportStatus();
    };
    
  } catch (e) {
    console.error("Transport initialization failed:", e);
    appendMessage("System", `❌ Transport init failed: ${e.message}`);
  }
}

// Auto-connect to a peer when they join the room
async function autoConnectToPeer(peerID) {
  if (peerID === peerId || peerConnections.has(peerID)) {
    return; // Don't connect to self or already connected peers
  }
  
  try {
    appendMessage("System", `🔄 Auto-connecting to ${peerID}...`);
    await createPeerConnection(peerID, true); // We initiate the connection
  } catch (e) {
    console.error(`Auto-connect to ${peerID} failed:`, e);
    appendMessage("System", `❌ Auto-connect to ${peerID} failed: ${e.message}`);
  }
}

// Handle signaling messages for WebRTC setup
async function handleSignalingMessage(signal) {
  try {
    const peerID = signal.peer_id;
    
    if (signal.type === 'offer') {
      // Handle incoming offer
      await handleOffer(peerID, signal.payload);
    } else if (signal.type === 'answer') {
      // Handle incoming answer
      await handleAnswer(peerID, signal.payload);
    } else if (signal.type === 'ice') {
      // Handle incoming ICE candidate
      await handleIceCandidate(peerID, signal.payload);
    }
  } catch (e) {
    console.error("Error handling signaling message:", e);
    appendMessage("System", `❌ WebRTC error: ${e.message}`);
  }
}

// Create a new peer connection with improved error handling
async function createPeerConnection(peerID, isInitiator = false) {
  console.log(`🔗 Creating peer connection to ${peerID}, initiator: ${isInitiator}`);
  
  const pc = new RTCPeerConnection(rtcConfig);
  peerConnections.set(peerID, pc);
  
  // Set a timeout for connection attempts
  const connectionTimeout = setTimeout(() => {
    if (pc.connectionState !== 'connected') {
      console.log(`⏰ Connection timeout for ${peerID}`);
      appendMessage("System", `⏰ Connection timeout with ${peerID}, cleaning up...`);
      pc.close();
      peerConnections.delete(peerID);
      connectedPeers.delete(peerID);
      updateTransportStatus();
    }
  }, 15000); // 15 second timeout
  
  // Handle ICE candidates
  pc.onicecandidate = (event) => {
    if (event.candidate) {
      console.log(`🧊 ICE candidate for ${peerID}:`, event.candidate.candidate);
      if (signalingWS && signalingWS.readyState === WebSocket.OPEN) {
        signalingWS.send(JSON.stringify({
          peer_id: peerId,
          target_peer_id: peerID,
          type: 'ice',
          payload: JSON.stringify(event.candidate)
        }));
      }
    } else {
      console.log(`🧊 ICE gathering complete for ${peerID}`);
    }
  };
  
  // Handle ICE connection state changes
  pc.oniceconnectionstatechange = () => {
    console.log(`🧊 ICE connection state with ${peerID}: ${pc.iceConnectionState}`);
    
    if (pc.iceConnectionState === 'connected' || pc.iceConnectionState === 'completed') {
      appendMessage("System", `🧊 ICE connected with ${peerID}`);
      clearTimeout(connectionTimeout);
    } else if (pc.iceConnectionState === 'failed') {
      appendMessage("System", `❌ ICE connection failed with ${peerID}`);
      clearTimeout(connectionTimeout);
      // Clean up failed connection
      setTimeout(() => {
        pc.close();
        peerConnections.delete(peerID);
        connectedPeers.delete(peerID);
        updateTransportStatus();
      }, 1000);
    }
  };
  
  // Handle incoming data channel
  pc.ondatachannel = (event) => {
    const channel = event.channel;
    console.log(`📡 Received data channel from ${peerID}`);
    setupDataChannel(peerID, channel);
  };
  
  // Handle connection state changes
  pc.onconnectionstatechange = () => {
    console.log(`🔗 WebRTC connection state with ${peerID}: ${pc.connectionState}`);
    appendMessage("System", `🔗 WebRTC ${pc.connectionState} with ${peerID}`);
    
    if (pc.connectionState === 'connected') {
      appendMessage("System", `🚀 WebRTC P2P connected to ${peerID}`);
      connectedPeers.add(peerID);
      transportStatus = 'connected';
      clearTimeout(connectionTimeout);
      updateTransportStatus();
    } else if (pc.connectionState === 'disconnected' || pc.connectionState === 'failed') {
      connectedPeers.delete(peerID);
      clearTimeout(connectionTimeout);
      updateTransportStatus();
      
      if (pc.connectionState === 'failed') {
        appendMessage("System", `❌ WebRTC connection to ${peerID} failed`);
        // Clean up after failure
        setTimeout(() => {
          pc.close();
          peerConnections.delete(peerID);
        }, 1000);
      }
    }
  };
  
  // If we're the initiator, create a data channel
  if (isInitiator) {
    console.log(`📡 Creating data channel as initiator for ${peerID}`);
    const channel = pc.createDataChannel('chat', { 
      ordered: true,
      maxRetransmits: 3
    });
    setupDataChannel(peerID, channel);
    
    // Create and send offer
    try {
      const offer = await pc.createOffer({
        offerToReceiveAudio: false,
        offerToReceiveVideo: false
      });
      await pc.setLocalDescription(offer);
      
      console.log(`📤 Sending offer to ${peerID}`);
      if (signalingWS && signalingWS.readyState === WebSocket.OPEN) {
        signalingWS.send(JSON.stringify({
          peer_id: peerId,
          target_peer_id: peerID,
          type: 'offer',
          payload: JSON.stringify(offer)
        }));
      }
    } catch (e) {
      console.error(`Failed to create offer for ${peerID}:`, e);
      clearTimeout(connectionTimeout);
      throw e;
    }
  }
  
  return pc;
}

// Setup data channel for messaging with improved state handling
function setupDataChannel(peerID, channel) {
  console.log(`📡 Setting up data channel with ${peerID}, initial state: ${channel.readyState}`);
  dataChannels.set(peerID, channel);
  
  // Add buffered amount tracking
  let lastBufferedAmount = 0;
  const bufferThreshold = 16384; // 16KB
  
  channel.onopen = () => {
    console.log(`📡 Data channel opened with ${peerID}`);
    appendMessage("System", `📡 ✅ Data channel ready with ${peerID} - WebRTC P2P active!`);
    updateTransportStatus();
    
    // Send a test message to verify the channel
    try {
      const testMessage = JSON.stringify({ 
        from: peerId, 
        text: `🔧 Connection test from ${peerId}`,
        type: 'test'
      });
      channel.send(testMessage);
      console.log(`✅ Test message sent to ${peerID}`);
    } catch (e) {
      console.warn(`Test message failed to ${peerID}:`, e);
    }
  };
  
  channel.onmessage = (event) => {
    console.log(`📨 Received WebRTC message from ${peerID}:`, event.data);
    try {
      const message = JSON.parse(event.data);
      
      if (message.type === 'test') {
        appendMessage("System", `🔧 ✅ Connection test from ${peerID} successful`);
        return;
      }
      
      // Regular message
      appendMessage(message.from || peerID, message.text);
      appendMessage("System", `📨 ✅ Message via WebRTC from ${peerID}`);
    } catch (e) {
      console.error("Error parsing data channel message:", e);
      appendMessage("System", `❌ Message parse error from ${peerID}`);
    }
  };
  
  channel.onerror = (error) => {
    console.error(`❌ Data channel error with ${peerID}:`, error);
    appendMessage("System", `❌ Data channel error with ${peerID}: ${error.type || 'unknown'}`);
  };
  
  channel.onclose = () => {
    console.log(`📡 Data channel closed with ${peerID}`);
    appendMessage("System", `📡 Data channel closed with ${peerID}`);
    dataChannels.delete(peerID);
    updateTransportStatus();
  };
  
  // Monitor buffer state
  const checkBuffer = () => {
    if (channel.readyState === 'open') {
      const bufferedAmount = channel.bufferedAmount;
      if (bufferedAmount > bufferThreshold && bufferedAmount > lastBufferedAmount) {
        console.warn(`⚠️  High buffer amount for ${peerID}: ${bufferedAmount} bytes`);
      }
      lastBufferedAmount = bufferedAmount;
    }
  };
  
  // Check buffer every 5 seconds
  const bufferInterval = setInterval(checkBuffer, 5000);
  
  // Clean up interval when channel closes
  const originalOnClose = channel.onclose;
  channel.onclose = (event) => {
    clearInterval(bufferInterval);
    if (originalOnClose) originalOnClose(event);
  };
}

// Handle incoming offer
async function handleOffer(peerID, offerData) {
  const offer = JSON.parse(offerData);
  const pc = await createPeerConnection(peerID, false);
  
  await pc.setRemoteDescription(offer);
  const answer = await pc.createAnswer();
  await pc.setLocalDescription(answer);
  
  if (signalingWS && signalingWS.readyState === WebSocket.OPEN) {
    signalingWS.send(JSON.stringify({
      peer_id: peerId,
      target_peer_id: peerID,
      type: 'answer',
      payload: JSON.stringify(answer)
    }));
  }
}

// Handle incoming answer
async function handleAnswer(peerID, answerData) {
  const answer = JSON.parse(answerData);
  const pc = peerConnections.get(peerID);
  if (pc) {
    await pc.setRemoteDescription(answer);
  }
}

// Handle incoming ICE candidate
async function handleIceCandidate(peerID, candidateData) {
  const candidate = JSON.parse(candidateData);
  const pc = peerConnections.get(peerID);
  if (pc) {
    await pc.addIceCandidate(candidate);
  }
}

// Send message via transport layer
async function sendViaTransport(peerID, text) {
  const res = await fetch(`${httpOrigin}/transport/send`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ peer_id: peerID, text, room }),
  });
  
  if (!res.ok) {
    throw new Error(`Transport send failed: ${res.statusText}`);
  }
  
  return res.json();
}

// Connect to a specific peer
async function connectToPeer() {
  const targetPeerID = document.getElementById("peer-id-input").value.trim();
  if (!targetPeerID) {
    alert("Please enter a peer ID to connect to");
    return;
  }
  
  try {
    appendMessage("System", `🔄 Initiating WebRTC connection to ${targetPeerID}...`);
    
    // Create WebRTC peer connection as initiator
    await createPeerConnection(targetPeerID, true);
    
    // Also try transport layer connection
    const res = await fetch(`${httpOrigin}/transport/connect`, {
      method: "POST", 
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ peer_id: targetPeerID, room }),
    });
    
    if (!res.ok) throw new Error(res.statusText);
    
  } catch (e) {
    appendMessage("System", `❌ Connection failed: ${e.message}`);
  }
}

// Toggle transport layer on/off
function toggleTransport() {
  useTransportLayer = !useTransportLayer;
  const btn = document.getElementById("transport-toggle");
  btn.textContent = useTransportLayer ? "Disable Transport" : "Enable Transport";
  
  if (useTransportLayer && room) {
    initializeTransport();
  } else if (signalingWS) {
    signalingWS.close();
    connectedPeers.clear();
    transportStatus = 'disabled';
  }
  
  updateTransportStatus();
  appendMessage("System", `🔧 Transport layer ${useTransportLayer ? 'enabled' : 'disabled'}`);
}

// Update transport status display
function updateTransportStatus() {
  const statusElement = document.getElementById("transport-status");
  if (statusElement) {
    // Count open WebRTC channels
    let openChannels = 0;
    for (const [peerID, channel] of dataChannels) {
      if (channel.readyState === 'open') {
        openChannels++;
      }
    }
    
    statusElement.textContent = `Transport: ${transportStatus} | Peers: ${connectedPeers.size} | WebRTC Channels: ${openChannels}/${dataChannels.size}`;
    statusElement.className = `transport-status ${transportStatus}`;
    
    // Update peer ID display
    const peerIdElement = document.getElementById("my-peer-id");
    if (peerIdElement) {
      peerIdElement.textContent = peerId;
    }
    
    // Update connected peers list
    const peersListElement = document.getElementById("connected-peers-list");
    if (peersListElement) {
      peersListElement.textContent = connectedPeers.size > 0 ? Array.from(connectedPeers).join(", ") : "None";
    }
    
    // Update WebRTC channels info
    const channelsElement = document.getElementById("webrtc-channels-list");
    if (channelsElement) {
      const channelInfo = [];
      for (const [peerID, channel] of dataChannels) {
        channelInfo.push(`${peerID.substring(0,8)}...: ${channel.readyState}`);
      }
      channelsElement.textContent = channelInfo.length > 0 ? channelInfo.join(", ") : "None";
    }
    
    // Update connection quality
    const qualityElement = document.getElementById("connection-quality");
    if (qualityElement) {
      if (openChannels > 0) {
        qualityElement.textContent = `P2P Active (${openChannels} WebRTC channels)`;
        qualityElement.style.color = "green";
      } else if (connectedPeers.size > 0) {
        qualityElement.textContent = "Transport Layer (fallback)";
        qualityElement.style.color = "orange";
      } else {
        qualityElement.textContent = "HTTP Polling Only";
        qualityElement.style.color = "red";
      }
    }
  }
}
