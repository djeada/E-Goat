// web/chat.js

// Only the signaling port needs to come from Go
const { wsPort } = window.config;

// Derive the HTTP host:port and protocol directly from the page
const httpOrigin = location.origin;              // e.g. "http://localhost:8080"
const wsHost     = `${location.hostname}:${wsPort}`;  // e.g. "localhost:9000"

let room, peerId, myIP;
let sigWs, chatWs;

// Helper: read URL param
function getParam(name) {
  return new URLSearchParams(location.search).get(name);
}

// On load…
window.addEventListener("load", () => {
  peerId = crypto.randomUUID();
  fetchIP();

  // If deep-linked with ?room=…&peer_id=…, auto-join
  const urlRoom   = getParam("room");
  const urlPeerId = getParam("peer_id");
  if (urlRoom && urlPeerId) {
    peerId = urlPeerId;
    document.getElementById("room-input").value = urlRoom;
    joinRoom();
    return;
  }

  // Otherwise wait for user actions
  document.getElementById("create-room-btn")
          .addEventListener("click", joinRoom);
  document.getElementById("send-btn")
          .addEventListener("click", sendMessage);
});

// 1 Get your external IP
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

// 2 Create or join the room
function joinRoom() {
  room = document.getElementById("room-input").value.trim();
  if (!room) {
    return alert("Please enter a room name.");
  }

  // Build the HTTP deep-link using location.origin
  const invite = `${httpOrigin}/?room=${room}&peer_id=${peerId}`;
  document.getElementById("invite-text"     ).value = invite;
  document.getElementById("invite-text-chat").value = invite;
  document.getElementById("invitation").classList.remove("hidden");

  // Swap UI
  document.getElementById("init").classList.add("hidden");
  document.getElementById("chat").classList.remove("hidden");
  document.getElementById("room-info").textContent = `Room: ${room}`;

  // Open the two WebSocket channels
  openSignalingWS();
  openChatWS();
}

// 3 Signaling WS (Pion handshake) at separate port
function openSignalingWS() {
  sigWs = new WebSocket(`ws://${wsHost}/signal?room=${room}&peer_id=${peerId}`);
  sigWs.addEventListener("open", () => console.log("🔑 Signaling open"));
  sigWs.addEventListener("message", evt => console.debug("⏳ Signal:", evt.data));
}

// 4 Chat WS (text) on the same origin/port as the page
function openChatWS() {
  chatWs = new WebSocket(`${httpOrigin.replace(/^http/, "ws")}/chat?room=${room}&peer_id=${peerId}`);
  chatWs.addEventListener("open", () => console.log("💬 Chat open"));
  chatWs.addEventListener("message", evt => {
    const msg = JSON.parse(evt.data);
    // Only render others’ messages (we render ours on send)
    if (msg.peer_id !== peerId) {
      appendMessage(msg.peer_id, msg.text);
    }
  });
}

// 5 Send a chat message
function sendMessage() {
  const input = document.getElementById("msg-input");
  const text  = input.value.trim();
  if (!text || !chatWs || chatWs.readyState !== WebSocket.OPEN) return;
  chatWs.send(JSON.stringify({ peer_id: peerId, text }));
  appendMessage("Me", text);
  input.value = "";
}

// 6️Append to chat log
function appendMessage(from, txt) {
  const container = document.getElementById("messages");
  const line = document.createElement("div");
  line.textContent = `${from}: ${txt}`;
  container.appendChild(line);
  container.scrollTop = container.scrollHeight;
}
