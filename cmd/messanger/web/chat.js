// web/chat.js

// Grab only the signaling port from config (chat uses the same host:port as the page)
const { wsPort } = window.config;
const host    = location.hostname;
const httpHost = location.host;     // includes port, e.g. "localhost:8080"

let room, peerId, myIP;
let sigWs, chatWs;

// Helper to read URL query params
function getParam(name) {
  return new URLSearchParams(location.search).get(name);
}

// On load…
window.addEventListener("load", () => {
  peerId = crypto.randomUUID();
  fetchIP();

  // Auto-join if the URL has room & peer_id
  const urlRoom   = getParam("room");
  const urlPeerId = getParam("peer_id");
  if (urlRoom && urlPeerId) {
    peerId = urlPeerId;
    document.getElementById("room-input").value = urlRoom;
    joinRoom();
    return;
  }

  document.getElementById("create-room-btn")
          .addEventListener("click", joinRoom);
  document.getElementById("send-btn")
          .addEventListener("click", sendMessage);
});

// 1️Fetch your external IP for display
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

// 2️ Create/join a room
function joinRoom() {
  room = document.getElementById("room-input").value.trim();
  if (!room) return alert("Please enter a room name.");

  // Build HTTP deep-link invitation
  const invite = `http://${httpHost}/?room=${room}&peer_id=${peerId}`;
  document.getElementById("invite-text"     ).value = invite;
  document.getElementById("invite-text-chat").value = invite;
  document.getElementById("invitation").classList.remove("hidden");

  // Show chat UI
  document.getElementById("init").classList.add("hidden");
  document.getElementById("chat").classList.remove("hidden");
  document.getElementById("room-info").textContent = `Room: ${room}`;

  openSignalingWS();
  openChatWS();
}

// 3️ Signaling WS for WebRTC handshake
function openSignalingWS() {
  sigWs = new WebSocket(
    `ws://${host}:${wsPort}/signal?room=${room}&peer_id=${peerId}`
  );
  sigWs.addEventListener("open", () => console.log("🔑 Signaling WS open"));
  sigWs.addEventListener("message", evt => console.debug("⏳ Signal:", evt.data));
}

// 4️ Chat WS for text messages (same host:port as HTTP)
function openChatWS() {
  chatWs = new WebSocket(
    `ws://${httpHost}/chat?room=${room}&peer_id=${peerId}`
  );
  chatWs.addEventListener("open", () => console.log("💬 Chat WS open"));
  chatWs.addEventListener("message", evt => {
    const msg = JSON.parse(evt.data);
    // Only show from others (we render ours on send)
    if (msg.peer_id !== peerId) {
      appendMessage(msg.peer_id, msg.text);
    }
  });
}

// 5️ Send a chat message
function sendMessage() {
  const input = document.getElementById("msg-input");
  const text  = input.value.trim();
  if (!text || !chatWs || chatWs.readyState !== WebSocket.OPEN) return;
  chatWs.send(JSON.stringify({ peer_id: peerId, text }));
  appendMessage("Me", text);
  input.value = "";
}

// 6️ Append a line to the chat box
function appendMessage(from, txt) {
  const container = document.getElementById("messages");
  const line = document.createElement("div");
  line.textContent = `${from}: ${txt}`;
  container.appendChild(line);
  container.scrollTop = container.scrollHeight;
}
