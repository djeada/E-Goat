// web/chat.js

// Grab the signaling port (still used for WebRTC, if you want it)
const { wsPort } = window.config;

// Derive our HTTP origin (e.g. "http://localhost:8080")
const httpOrigin = location.origin;

let room, peerId, myIP;
let pollInterval;
let lastTs = 0;  // UNIX timestamp of the last‐seen message

window.addEventListener("load", () => {
  peerId = crypto.randomUUID();
  fetchIP();

  // Auto-join if deep-linked
  const params = new URLSearchParams(location.search);
  const urlRoom   = params.get("room");
  const urlPeerId = params.get("peer_id");
  if (urlRoom && urlPeerId) {
    peerId = urlPeerId;
    document.getElementById("room-input").value = urlRoom;
    joinRoom();
  } else {
    document.getElementById("create-room-btn")
            .addEventListener("click", joinRoom);
    document.getElementById("send-btn")
            .addEventListener("click", sendMessage);
  }
});

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

function joinRoom() {
  room = document.getElementById("room-input").value.trim();
  if (!room) return alert("Please enter a room name.");

  // Show invite deep-link
  const invite = `${httpOrigin}/?room=${room}&peer_id=${peerId}`;
  document.getElementById("invite-text"     ).value = invite;
  document.getElementById("invite-text-chat").value = invite;
  document.getElementById("invitation").classList.remove("hidden");

  // Swap views
  document.getElementById("init").classList.add("hidden");
  document.getElementById("chat").classList.remove("hidden");
  document.getElementById("room-info").textContent = `Room: ${room}`;

  // Start polling history
  pollHistory();
  pollInterval = setInterval(pollHistory, 1000);
}

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

async function sendMessage() {
  const input = document.getElementById("msg-input");
  const text  = input.value.trim();
  if (!text) return;

  appendMessage("Me", text);
  input.value = "";

  try {
    const res = await fetch(`${httpOrigin}/send`, {
      method:  "POST",
      headers: { "Content-Type": "application/json" },
      body:    JSON.stringify({ room, peer_id: peerId, text }),
    });
    if (!res.ok) throw new Error(res.statusText);
    // Optionally read returned timestamp:
    const { timestamp } = await res.json();
    if (timestamp > lastTs) lastTs = timestamp;
  } catch (e) {
    // Mark as failed
    appendMessage("Error", text);
    console.error("Send failed:", e);
  }
}

function appendMessage(from, txt) {
  const container = document.getElementById("messages");
  const line = document.createElement("div");
  line.textContent = `${from}: ${txt}`;
  container.appendChild(line);
  container.scrollTop = container.scrollHeight;
}
