// web/chat.js

// We support both HTTP polling and WebRTC connections
const config = window.config || {};
const httpOrigin = location.origin;
const wsPort = Number.isInteger(config.wsPort) ? config.wsPort : 9000;
const wsOrigin = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.hostname}:${wsPort}`;

function normalizePublicBase(value) {
  if (!value || typeof value !== 'string') return httpOrigin;
  let base = value.trim();
  if (!base) return httpOrigin;
  if (!base.includes("://")) {
    base = `${location.protocol}//${base}`;
  }
  try {
    const url = new URL(base);
    return url.origin;
  } catch {
    return httpOrigin;
  }
}

const publicBase = normalizePublicBase(config.publicBase);

// Display configuration
const PEER_ID_TRUNCATE_LENGTH = 8; // Number of characters to show for truncated peer IDs

// Common emojis for the picker
const EMOJI_LIST = [
  '😀', '😃', '😄', '😁', '😆', '😅', '🤣', '😂',
  '🙂', '😉', '😊', '😇', '🥰', '😍', '🤩', '😘',
  '😋', '😛', '😜', '🤪', '😝', '🤑', '🤗', '🤔',
  '🤐', '🤨', '😐', '😑', '😶', '😏', '😒', '🙄',
  '😬', '😮', '🤯', '😱', '😨', '😰', '😥', '😢',
  '😭', '😤', '😡', '🤬', '😈', '👿', '💀', '☠️',
  '👋', '🤚', '🖐️', '✋', '🖖', '👌', '🤌', '🤏',
  '✌️', '🤞', '🤟', '🤘', '🤙', '👈', '👉', '👆',
  '👍', '👎', '✊', '👊', '🤛', '🤜', '👏', '🙌',
  '❤️', '🧡', '💛', '💚', '💙', '💜', '🖤', '🤍',
  '💯', '💢', '💥', '💫', '💦', '💨', '🔥', '⭐',
  '🎉', '🎊', '🎁', '🎈', '🚀', '✨', '🌟', '💡'
];

let room, peerId;
let pollInterval;
let lastTs = 0;  // UNIX timestamp of the last‐seen message
let typingTimeout = null;
let isTyping = false;

// WebRTC and Transport variables
let signalingWS = null;
let useTransportLayer = true;
let connectedPeers = new Set();
let transportStatus = 'disconnected';
let peerConnections = new Map(); // Store WebRTC peer connections
let dataChannels = new Map(); // Store WebRTC data channels
let messageCount = 0; // Track messages for unique IDs

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
  document.getElementById("apply-strategy-btn")
          .addEventListener("click", applyTransportStrategy);
  document.getElementById("transport-strategy-select")
          .addEventListener("change", updateStrategyDescription);
  document.getElementById("transport-strategy-select")
          .addEventListener("change", updateApplyButtonState);
  
  // Help toggle button
  const helpToggleBtn = document.getElementById("help-toggle-btn");
  if (helpToggleBtn) helpToggleBtn.addEventListener("click", toggleConnectionHelp);
  
  // Copy buttons
  const copyBtn = document.getElementById("copy-invite-btn");
  if (copyBtn) copyBtn.addEventListener("click", () => copyToClipboard("invite-text", copyBtn));
  
  const copyChatBtn = document.getElementById("copy-invite-chat-btn");
  if (copyChatBtn) copyChatBtn.addEventListener("click", () => copyToClipboard("invite-text-chat", copyChatBtn));
  
  // Emoji picker
  const emojiBtn = document.getElementById("emoji-btn");
  if (emojiBtn) emojiBtn.addEventListener("click", toggleEmojiPicker);
  
  const closeEmojiBtn = document.getElementById("close-emoji-picker");
  if (closeEmojiBtn) closeEmojiBtn.addEventListener("click", () => {
    document.getElementById("emoji-picker").classList.add("hidden");
  });
  
  // Initialize emoji grid
  initializeEmojiPicker();
  
  // Keyboard shortcuts
  document.getElementById("room-input")
          .addEventListener("keypress", (e) => {
            if (e.key === "Enter") joinRoom();
          });
  document.getElementById("msg-input")
          .addEventListener("keypress", (e) => {
            if (e.key === "Enter") sendMessage();
          });
  document.getElementById("msg-input")
          .addEventListener("input", handleTyping);
  document.getElementById("peer-id-input")
          .addEventListener("keypress", (e) => {
            if (e.key === "Enter") connectToPeer();
          });
  
  // Close emoji picker when clicking outside
  document.addEventListener("click", (e) => {
    const picker = document.getElementById("emoji-picker");
    const emojiBtn = document.getElementById("emoji-btn");
    if (picker && !picker.contains(e.target) && e.target !== emojiBtn) {
      picker.classList.add("hidden");
    }
  });
});

// 1 Get your external IP for display
async function fetchIP() {
  try {
    const res = await fetch("https://api.ipify.org?format=json");
    const { ip } = await res.json();
    myIP = ip;
    document.getElementById("my-ip").textContent = ip;
    updateWanWarning(ip);
  } catch {
    document.getElementById("my-ip").textContent = "Unknown";
  }
}

function updateWanWarning(ip) {
  const warningEl = document.getElementById("wan-warning");
  if (!warningEl) return;
  if (isPrivateIP(ip)) {
    warningEl.textContent = "CGNAT detected: internet P2P may fail without TURN";
    warningEl.classList.remove("hidden");
  } else {
    warningEl.textContent = "";
    warningEl.classList.add("hidden");
  }
}

function isPrivateIP(ip) {
  if (!ip) return false;
  if (ip.startsWith("10.")) return true;
  if (ip.startsWith("192.168.")) return true;
  if (ip.startsWith("172.")) {
    const parts = ip.split(".");
    if (parts.length > 1) {
      const second = parseInt(parts[1], 10);
      if (second >= 16 && second <= 31) return true;
    }
  }
  if (ip.startsWith("100.")) {
    const parts = ip.split(".");
    if (parts.length > 1) {
      const second = parseInt(parts[1], 10);
      if (second >= 64 && second <= 127) return true;
    }
  }
  return false;
}

// 2 Create/join a room
function joinRoom() {
  room = document.getElementById("room-input").value.trim();
  if (!room) {
    return alert("Please enter a room name.");
  }

  // Build and show the HTTP deep‐link invite
  const invite = `${publicBase}/?room=${room}&peer_id=${peerId}`;
  document.getElementById("invite-text"     ).value = invite;
  document.getElementById("invite-text-chat").value = invite;
  document.getElementById("invitation").classList.remove("hidden");
  updateInviteWarning();

  // Swap to chat UI
  document.getElementById("init").classList.add("hidden");
  document.getElementById("chat").classList.remove("hidden");
  
  // Update room name display (new modern UI)
  const roomNameDisplay = document.getElementById("room-name-display");
  if (roomNameDisplay) {
    roomNameDisplay.textContent = room;
  }

  // Start polling for new messages once per second
  pollHistory();
  pollInterval = setInterval(pollHistory, 1000);
  
  // Initialize transport layer if enabled
  if (useTransportLayer) {
    initializeTransport();
  }

  fetchTransportOptions();
  
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
    // Handle null or non-array responses
    if (!messages || !Array.isArray(messages)) return;
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

  // Optimistic UI with proper "Me" styling
  appendMessage("Me", text, { isMe: true });
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

// 5 Helper to append a line to the chat box with enhanced formatting
function appendMessage(from, txt, options = {}) {
  const container = document.getElementById("messages");
  const { via = null, timestamp = null, isMe = false } = options;
  
  messageCount++;
  
  // Create message container
  const messageDiv = document.createElement("div");
  messageDiv.className = "message";
  messageDiv.id = `msg-${messageCount}`;
  
  // Determine message type for styling
  let avatarClass = "";
  let senderClass = "";
  let avatarContent = "";
  
  if (from === "Me" || isMe) {
    avatarClass = "me";
    senderClass = "me";
    avatarContent = "👤";
  } else if (from === "System") {
    avatarClass = "system";
    senderClass = "system";
    avatarContent = "⚙️";
  } else if (from === "Error") {
    avatarClass = "error";
    senderClass = "error";
    avatarContent = "⚠️";
  } else {
    avatarContent = from.charAt(0).toUpperCase();
    // Add webrtc class if sent via WebRTC
    if (via === "webrtc") {
      messageDiv.classList.add("webrtc");
    }
  }
  
  // Create avatar
  const avatar = document.createElement("div");
  avatar.className = `message-avatar ${avatarClass}`;
  avatar.textContent = avatarContent;
  
  // Create content container
  const content = document.createElement("div");
  content.className = "message-content";
  
  // Create header with sender name and optional timestamp
  const header = document.createElement("div");
  header.className = "message-header";
  
  const sender = document.createElement("span");
  sender.className = `message-sender ${senderClass}`;
  sender.textContent = from;
  header.appendChild(sender);
  
  // Add via indicator for WebRTC messages
  if (via) {
    const viaSpan = document.createElement("span");
    viaSpan.className = "message-via";
    viaSpan.textContent = via === "webrtc" ? "P2P" : via;
    header.appendChild(viaSpan);
  }
  
  // Add timestamp
  const time = document.createElement("span");
  time.className = "message-timestamp";
  const now = timestamp ? new Date(timestamp * 1000) : new Date();
  time.textContent = now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  header.appendChild(time);
  
  // Create message text
  const text = document.createElement("div");
  text.className = "message-text";
  text.textContent = txt;
  
  // Assemble message
  content.appendChild(header);
  content.appendChild(text);
  messageDiv.appendChild(avatar);
  messageDiv.appendChild(content);
  
  container.appendChild(messageDiv);
  container.scrollTop = container.scrollHeight;
  
  // Play notification sound for incoming messages (not from Me or System)
  if (from !== "Me" && from !== "System" && from !== "Error" && !isMe) {
    playNotificationSound();
  }
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
          appendMessage("System", `👋 ${signal.peer_id.substring(0, 8)}... joined the room`);
          setTimeout(() => autoConnectToPeer(signal.peer_id), 1000);
        } else if (signal.type === 'typing') {
          // Handle typing indicator
          try {
            const typingData = JSON.parse(signal.payload);
            showTypingIndicator(signal.peer_id, typingData.typing);
          } catch (e) {
            console.error("Failed to parse typing signal:", e);
          }
        } else {
          // Don't show verbose signal messages for offer/answer/ice
          if (!['offer', 'answer', 'ice'].includes(signal.type)) {
            appendMessage("System", `📡 Signal from ${signal.peer_id.substring(0, 8)}...: ${signal.type}`);
          }
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
    appendMessage("System", `🔄 Auto-connecting to ${peerID.substring(0, 8)}...`);
    await createPeerConnection(peerID, true); // We initiate the connection
  } catch (e) {
    console.error(`Auto-connect to ${peerID} failed:`, e);
    appendMessage("System", `❌ Auto-connect to ${peerID.substring(0, 8)}... failed`);
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
      appendMessage("System", `⏰ Connection timeout with ${peerID.substring(0, 8)}...`);
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
      appendMessage("System", `🧊 ICE connected with ${peerID.substring(0, 8)}...`);
      clearTimeout(connectionTimeout);
    } else if (pc.iceConnectionState === 'failed') {
      appendMessage("System", `❌ ICE connection failed with ${peerID.substring(0, 8)}...`);
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
    // Only show connecting state once, not every state change
    if (pc.connectionState !== 'connecting') {
      appendMessage("System", `🔗 WebRTC ${pc.connectionState} with ${peerID.substring(0, 8)}...`);
    }
    
    if (pc.connectionState === 'connected') {
      appendMessage("System", `🚀 WebRTC P2P connected to ${peerID.substring(0, 8)}...`);
      connectedPeers.add(peerID);
      transportStatus = 'connected';
      clearTimeout(connectionTimeout);
      updateTransportStatus();
    } else if (pc.connectionState === 'disconnected' || pc.connectionState === 'failed') {
      connectedPeers.delete(peerID);
      clearTimeout(connectionTimeout);
      updateTransportStatus();
      
      if (pc.connectionState === 'failed') {
        appendMessage("System", `❌ WebRTC connection to ${peerID.substring(0, 8)}... failed`);
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
    appendMessage("System", `📡 ✅ P2P channel ready with ${peerID.substring(0, 8)}... - WebRTC active!`);
    updateTransportStatus();
    
    // Send a test message to verify the channel
    try {
      const testMessage = JSON.stringify({ 
        from: peerId, 
        text: `🔧 Connection test`,
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
        appendMessage("System", `🔧 ✅ Connection test from ${peerID.substring(0, 8)}... successful`);
        return;
      }
      
      // Regular message with enhanced formatting
      const senderName = message.from ? 
        (message.from.length > 12 ? message.from.substring(0, 8) + '...' : message.from) : 
        peerID.substring(0, 8) + '...';
      appendMessage(senderName, message.text, { via: 'webrtc' });
    } catch (e) {
      console.error("Error parsing data channel message:", e);
      appendMessage("System", `❌ Message parse error from ${peerID.substring(0, 8)}...`);
    }
  };
  
  channel.onerror = (error) => {
    console.error(`❌ Data channel error with ${peerID}:`, error);
    appendMessage("System", `❌ Data channel error with ${peerID.substring(0, 8)}...`);
  };
  
  channel.onclose = () => {
    console.log(`📡 Data channel closed with ${peerID}`);
    appendMessage("System", `📡 Data channel closed with ${peerID.substring(0, 8)}...`);
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
    
    // Build status message with icon
    let statusIcon = '⚡';
    let statusText = '';
    
    // Get current strategy for display
    const currentStrategy = window.currentStrategyName || 'auto';
    
    if (!useTransportLayer) {
      statusIcon = '⏸️';
      statusText = 'Transport Disabled - Using HTTP polling only';
    } else if (openChannels > 0) {
      statusIcon = '🚀';
      statusText = `P2P Active via WebRTC • ${openChannels} Channel${openChannels > 1 ? 's' : ''} • ${connectedPeers.size} Peer${connectedPeers.size > 1 ? 's' : ''}`;
    } else if (transportStatus === 'signaling_connected') {
      statusIcon = '✅';
      // More descriptive message explaining setup is complete
      statusText = `Setup Complete (${currentStrategy}) • Share invite link for others to join • Will use best available method`;
    } else if (transportStatus === 'connected') {
      statusIcon = '✅';
      statusText = `Connected via Transport • ${connectedPeers.size} Peer${connectedPeers.size > 1 ? 's' : ''}`;
    } else if (transportStatus === 'error') {
      statusIcon = '❌';
      statusText = 'Connection Error - Falling back to HTTP polling';
    } else {
      statusIcon = '📡';
      statusText = 'Initializing connection...';
    }
    
    statusElement.textContent = `${statusIcon} ${statusText}`;
    statusElement.className = `transport-status ${transportStatus}`;
    
    // Update status explanation with more context
    const statusDetail = document.getElementById("status-detail");
    if (statusDetail) {
      if (!useTransportLayer) {
        statusDetail.textContent = "Transport layer is disabled. Messages will be sent via HTTP polling (higher latency).";
      } else if (openChannels > 0) {
        statusDetail.textContent = `Direct peer-to-peer connection established via WebRTC. This is the fastest connection method with lowest latency.`;
      } else if (transportStatus === 'signaling_connected') {
        statusDetail.textContent = `Your room is ready! Share the invite link below. When someone joins, E-Goat will automatically connect using the best available method (${currentStrategy} strategy).`;
      } else if (transportStatus === 'connected') {
        statusDetail.textContent = `Connected to ${connectedPeers.size} peer(s) via the transport layer.`;
      } else if (transportStatus === 'error') {
        statusDetail.textContent = "Connection failed. Using HTTP polling as fallback. Try refreshing the page.";
      } else {
        statusDetail.textContent = "Connecting to signaling server...";
      }
    }

    const strategyName = document.getElementById("strategy-name");
    if (strategyName && window.currentStrategyName) {
      strategyName.textContent = window.currentStrategyName;
    }
    
    // Update peer ID display (truncated for readability)
    const peerIdElement = document.getElementById("my-peer-id");
    if (peerIdElement) {
      peerIdElement.textContent = peerId ? `${peerId.substring(0, PEER_ID_TRUNCATE_LENGTH)}...` : 'Generating...';
      peerIdElement.title = peerId; // Full ID on hover
    }
    
    // Update connected peers list
    const peersListElement = document.getElementById("connected-peers-list");
    if (peersListElement) {
      if (connectedPeers.size > 0) {
        const truncatedPeers = Array.from(connectedPeers).map(p => `${p.substring(0, PEER_ID_TRUNCATE_LENGTH)}...`);
        peersListElement.textContent = truncatedPeers.join(', ');
      } else {
        peersListElement.textContent = 'None';
      }
    }
    
    // Update WebRTC channels info
    const channelsElement = document.getElementById("webrtc-channels-list");
    if (channelsElement) {
      const channelInfo = [];
      for (const [peerID, channel] of dataChannels) {
        const stateEmoji = channel.readyState === 'open' ? '🟢' : 
                           channel.readyState === 'connecting' ? '🟡' : '🔴';
        channelInfo.push(`${stateEmoji} ${peerID.substring(0, PEER_ID_TRUNCATE_LENGTH - 2)}...`);
      }
      channelsElement.textContent = channelInfo.length > 0 ? channelInfo.join(' | ') : 'None';
    }
    
    // Update connection quality with color-coded CSS and more context
    const qualityElement = document.getElementById("connection-quality");
    if (qualityElement) {
      const currentStrategy = window.currentStrategyName || 'auto';
      if (openChannels > 0) {
        qualityElement.textContent = `🟢 P2P Active (WebRTC - ${openChannels} channel${openChannels > 1 ? 's' : ''})`;
        qualityElement.style.color = "#38ef7d";
      } else if (connectedPeers.size > 0) {
        qualityElement.textContent = "🟡 Connected via Transport Layer";
        qualityElement.style.color = "#f5576c";
      } else if (transportStatus === 'signaling_connected') {
        qualityElement.textContent = `🟢 Ready (${currentStrategy}) - Awaiting peers`;
        qualityElement.style.color = "#38ef7d";
      } else {
        qualityElement.textContent = "🔴 HTTP Polling Only";
        qualityElement.style.color = "#ff416c";
      }
    }
    
    // Update room peer count with more context
    const peerCountElement = document.getElementById("room-peer-count");
    if (peerCountElement) {
      const totalPeers = connectedPeers.size;
      if (totalPeers > 0) {
        peerCountElement.textContent = `${totalPeers} peer${totalPeers > 1 ? 's' : ''} connected`;
      } else {
        peerCountElement.textContent = 'Ready - Share invite to connect';
      }
    }
  }
}

async function fetchTransportOptions() {
  try {
    const res = await fetch(`${httpOrigin}/transport/options`);
    if (!res.ok) throw new Error(res.statusText);
    const data = await res.json();
    renderTransportOptions(data);
  } catch (e) {
    console.error("Failed to load transport options:", e);
  }
}

function renderTransportOptions(data) {
  const select = document.getElementById("transport-strategy-select");
  const description = document.getElementById("strategy-description");
  const available = document.getElementById("available-transports");

  if (!select || !data) return;
  select.innerHTML = "";

  const strategies = Array.isArray(data.strategies) ? data.strategies : [];
  const current = data.current_strategy || "";
  window.currentStrategyName = current || "auto";
  window.strategyCatalog = strategies;

  strategies.sort((a, b) => a.name.localeCompare(b.name));
  for (const strategy of strategies) {
    const opt = document.createElement("option");
    opt.value = strategy.name;
    opt.textContent = strategy.name;
    if (strategy.name === current) {
      opt.selected = true;
      if (description) description.textContent = strategy.description || "";
    }
    select.appendChild(opt);
  }

  updateStrategyDescription();
  updateApplyButtonState();

  if (available) {
    const transports = Array.isArray(data.transports) ? data.transports : [];
    available.textContent = "";
    for (const transport of transports) {
      const chip = document.createElement("span");
      chip.className = `transport-chip${transport.enabled ? "" : " disabled"}`;
      chip.textContent = `${transport.type} ${transport.estimated_success}%`;
      available.appendChild(chip);
    }
    if (transports.length === 0) {
      available.textContent = "Unknown";
    }
  }

  updateTransportStatus();
}

async function applyTransportStrategy() {
  const select = document.getElementById("transport-strategy-select");
  const description = document.getElementById("strategy-description");
  if (!select) return;

  const name = select.value;
  try {
    const res = await fetch(`${httpOrigin}/transport/strategy`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
    });
    if (!res.ok) throw new Error(await res.text());
    window.currentStrategyName = name;
    if (description) {
      updateStrategyDescription();
    }
    await fetchTransportOptions();
    appendMessage("System", `🔧 Strategy set to ${name}`);
  } catch (e) {
    console.error("Failed to apply strategy:", e);
    appendMessage("System", `❌ Failed to set strategy: ${e.message}`);
  }
}

function updateStrategyDescription() {
  const select = document.getElementById("transport-strategy-select");
  const description = document.getElementById("strategy-description");
  if (!select || !description) return;

  const strategies = window.strategyCatalog || [];
  const match = strategies.find((s) => s.name === select.value);
  description.textContent = match ? match.description || "" : "";
}

function updateApplyButtonState() {
  const select = document.getElementById("transport-strategy-select");
  const button = document.getElementById("apply-strategy-btn");
  if (!select || !button) return;

  const current = window.currentStrategyName || "";
  button.disabled = select.value === current;
  button.textContent = button.disabled ? "Applied" : "Apply";
}

function updateInviteWarning() {
  const warning = document.getElementById("invite-warning");
  const warningText = warning?.querySelector(".warning-text");
  const inviteHelp = document.getElementById("invite-help");
  if (!warning) return;

  let host = location.hostname;
  try {
    host = new URL(publicBase).hostname;
  } catch {
    // fall back to current host
  }

  const isLocalOnly = (host === "localhost" || host === "127.0.0.1" || host === "::1");
  
  if (isLocalOnly) {
    if (warningText) {
      warningText.textContent = "This invite link only works on your local network.";
    } else {
      warning.textContent = "This invite link only works on your local network.";
    }
    warning.classList.remove("hidden");
    if (inviteHelp) inviteHelp.classList.remove("hidden");
  } else {
    if (warningText) {
      warningText.textContent = "";
    } else {
      warning.textContent = "";
    }
    warning.classList.add("hidden");
    if (inviteHelp) inviteHelp.classList.add("hidden");
  }
}

// Toggle connection help section visibility
function toggleConnectionHelp() {
  const helpSection = document.getElementById("connection-help");
  const helpBtn = document.getElementById("help-toggle-btn");
  if (!helpSection) return;
  
  const isHidden = helpSection.classList.contains("hidden");
  helpSection.classList.toggle("hidden");
  
  if (helpBtn) {
    helpBtn.textContent = isHidden ? "✕" : "❓";
    helpBtn.setAttribute("data-tooltip", isHidden ? "Hide help" : "Show help");
  }
}

// === NEW ENHANCED FEATURES ===

// Copy to clipboard functionality
async function copyToClipboard(textareaId, button) {
  const textarea = document.getElementById(textareaId);
  if (!textarea) return;
  
  try {
    await navigator.clipboard.writeText(textarea.value);
    
    // Visual feedback
    const originalText = button.textContent;
    button.textContent = "✅";
    button.classList.add("copied");
    
    setTimeout(() => {
      button.textContent = originalText;
      button.classList.remove("copied");
    }, 2000);
  } catch (err) {
    console.error("Failed to copy:", err);
    // Fallback for older browsers
    textarea.select();
    document.execCommand('copy');
    button.textContent = "✅";
    setTimeout(() => {
      button.textContent = "📋";
    }, 2000);
  }
}

// Initialize emoji picker
function initializeEmojiPicker() {
  const grid = document.getElementById("emoji-grid");
  if (!grid) return;
  
  for (const emoji of EMOJI_LIST) {
    const btn = document.createElement("button");
    btn.className = "emoji-btn";
    btn.textContent = emoji;
    btn.addEventListener("click", () => insertEmoji(emoji));
    grid.appendChild(btn);
  }
}

// Toggle emoji picker visibility
function toggleEmojiPicker() {
  const picker = document.getElementById("emoji-picker");
  if (picker) {
    picker.classList.toggle("hidden");
  }
}

// Insert emoji into message input
function insertEmoji(emoji) {
  const input = document.getElementById("msg-input");
  if (input) {
    const start = input.selectionStart;
    const end = input.selectionEnd;
    const value = input.value;
    input.value = value.slice(0, start) + emoji + value.slice(end);
    input.focus();
    input.setSelectionRange(start + emoji.length, start + emoji.length);
  }
  
  // Close picker after selection
  const picker = document.getElementById("emoji-picker");
  if (picker) {
    picker.classList.add("hidden");
  }
}

// Handle typing indicator
function handleTyping() {
  if (!signalingWS || signalingWS.readyState !== WebSocket.OPEN || !peerId) return;
  
  if (!isTyping) {
    isTyping = true;
    // Send typing start notification
    signalingWS.send(JSON.stringify({
      peer_id: peerId,
      type: 'typing',
      payload: JSON.stringify({ typing: true })
    }));
  }
  
  // Clear previous timeout
  if (typingTimeout) {
    clearTimeout(typingTimeout);
  }
  
  // Set timeout to stop typing after 2 seconds of inactivity
  typingTimeout = setTimeout(() => {
    isTyping = false;
    if (signalingWS && signalingWS.readyState === WebSocket.OPEN && peerId) {
      signalingWS.send(JSON.stringify({
        peer_id: peerId,
        type: 'typing',
        payload: JSON.stringify({ typing: false })
      }));
    }
  }, 2000);
}

// Show typing indicator from another peer
function showTypingIndicator(peerID, show) {
  const indicator = document.getElementById("typing-indicator");
  const typingText = indicator?.querySelector(".typing-text");
  
  if (indicator && typingText) {
    if (show && peerID) {
      typingText.textContent = `${peerID.substring(0, 8)}... is typing...`;
      indicator.classList.remove("hidden");
    } else {
      indicator.classList.add("hidden");
    }
  }
}

// Reusable audio context for notification sounds
let audioContext = null;

// Play notification sound for new messages
function playNotificationSound() {
  try {
    // Create or reuse audio context
    if (!audioContext) {
      audioContext = new (window.AudioContext || window.webkitAudioContext)();
    }
    
    // Resume context if suspended (browser autoplay policy)
    if (audioContext.state === 'suspended') {
      audioContext.resume();
    }
    
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();
    
    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);
    
    oscillator.frequency.value = 800; // Frequency in Hz
    oscillator.type = 'sine';
    
    gainNode.gain.setValueAtTime(0.1, audioContext.currentTime);
    gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.3);
    
    oscillator.start(audioContext.currentTime);
    oscillator.stop(audioContext.currentTime + 0.3);
  } catch (e) {
    // Silently fail if audio context is not available
    console.log("Audio notification not available");
  }
}
