// web/chat.js

// Grab the ports and host from the page context
const { wsPort, httpPort } = window.config;
const host = location.hostname;

let room, peerId, myIP;
let sigWs, chatWs;

// Helper to read URL query params
function getParam(name) {
  return new URLSearchParams(location.search).get(name);
}

// On load…
window.addEventListener("load", () => {
  // Generate our own peer ID
  peerId = crypto.randomUUID();
  fetchIP();

  // If the URL includes room & peer_id, auto‐join
  const urlRoom   = getParam("room");
  const urlPeerId = getParam("peer_id");
  if (urlRoom && urlPeerId) {
    // Use those values and start immediately
    peerId = urlPeerId;
    document.getElementById("room-input").value = urlRoom;
    joinRoom();
    return;
  }

  // Otherwise wait for the user to click
  document.getElementById("create-room-btn")
          .addEventListener("click", joinRoom);
  document.getElementById("send-btn")
          .addEventListener("click", sendMessage);
});

// 1. Fetch external IP for invitation
async function fetchIP() {
  try {
    const res = await fetch("https://api.ipify.org?format=json");
    const { ip } = await res.json();
    myIP = ip;
    document.getElementById("my-ip").textContent = ip;
  } catch (e) {
    console.error("IP fetch failed", e);
    document.getElementById("my-ip").textContent = "Unknown";
  }
}

// 2. Create/join a room
function joinRoom() {
  room = document.getElementById("room-input").value.trim();
  if (!room) return alert("Please enter a room name.");

  // Build invitation string (HTTP deep-link)
  const invite = `http://${myIP}:${httpPort}/?room=${room}&peer_id=${peerId}`;
  document.getElementById("invite-text").value      = invite;
  document.getElementById("invite-text-chat").value = invite;
  document.getElementById("invitation").classList.remove("hidden");

  // Swap views
  document.getElementById("init").classList.add("hidden");
  document.getElementById("chat").classList.remove("hidden");
  document.getElementById("room-info").textContent = `Room: ${room}`;

  // Open WebSockets
  openSignalingWS();
  openChatWS();
}

// 3. Signaling WS for WebRTC handshake (handled under-the-hood)
function openSignalingWS() {
  sigWs = new WebSocket(
    `ws://${host}:${wsPort}/signal?room=${room}&peer_id=${peerId}`
  );
  sigWs.addEventListener("open", () => {
    console.log("🔑 Signaling WS open");
  });
  sigWs.addEventListener("message", evt => {
    console.debug("⏳ Signal:", evt.data);
  });
}

// 4. Chat WS for text messages
function openChatWS() {
  chatWs = new WebSocket(
    `ws://${host}:${httpPort}/chat?room=${room}&peer_id=${peerId}`
  );
  chatWs.addEventListener("open", () => {
    console.log("💬 Chat WS open");
  });
  chatWs.addEventListener("message", evt => {
    const msg = JSON.parse(evt.data);
    // Only render messages from others
    if (msg.peer_id !== peerId) {
      appendMessage(msg.peer_id, msg.text);
    }
  });
}

// 5. Send a message
function sendMessage() {
  const input = document.getElementById("msg-input");
  const text  = input.value.trim();
  if (!text || !chatWs || chatWs.readyState !== WebSocket.OPEN) return;
  chatWs.send(JSON.stringify({ peer_id: peerId, text }));
  appendMessage("Me", text);
  input.value = "";
}

// 6. Append to chat area
function appendMessage(from, txt) {
  const container = document.getElementById("messages");
  const line = document.createElement("div");
  line.textContent = `${from}: ${txt}`;
  container.appendChild(line);
  container.scrollTop = container.scrollHeight;
}
