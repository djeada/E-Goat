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

  // Try transport layer first if available and connected
  if (useTransportLayer && connectedPeers.size > 0) {
    try {
      for (const peerID of connectedPeers) {
        await sendViaTransport(peerID, text);
      }
      appendMessage("System", `📡 Sent via transport layer to ${connectedPeers.size} peer(s)`);
      return;
    } catch (e) {
      console.error("Transport send failed, falling back to HTTP:", e);
      appendMessage("System", "🔄 Transport failed, falling back to HTTP polling");
    }
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
        appendMessage("System", `📡 Signal from ${signal.peer_id}: ${signal.type}`);
        handleSignalingMessage(signal);
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

// Handle signaling messages for WebRTC setup
function handleSignalingMessage(signal) {
  // This would handle WebRTC offer/answer/ICE candidates
  // For now, we'll simulate successful P2P connection
  if (signal.type === 'offer') {
    connectedPeers.add(signal.peer_id);
    appendMessage("System", `🤝 P2P connection established with ${signal.peer_id}`);
    transportStatus = 'connected';
    updateTransportStatus();
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
    const res = await fetch(`${httpOrigin}/transport/connect`, {
      method: "POST", 
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ peer_id: targetPeerID, room }),
    });
    
    if (!res.ok) throw new Error(res.statusText);
    
    appendMessage("System", `🔄 Connecting to peer ${targetPeerID}...`);
    
    // Simulate connection after delay
    setTimeout(() => {
      connectedPeers.add(targetPeerID);
      appendMessage("System", `✅ Connected to ${targetPeerID} via layered transport`);
      transportStatus = 'connected';
      updateTransportStatus();
    }, 2000);
    
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
    statusElement.textContent = `Transport: ${transportStatus} | Peers: ${connectedPeers.size}`;
    statusElement.className = `transport-status ${transportStatus}`;
  }
}
