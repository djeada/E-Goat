// web/chat.js

// We only need the HTTP origin; signaling WS is no longer used here
const httpOrigin = location.origin;

let room, peerId, myIP;
let pollInterval;
let lastTs = 0;  // UNIX timestamp of the last‐seen message

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

// 4 Send a new chat message via POST /send
async function sendMessage() {
  const input = document.getElementById("msg-input");
  const text  = input.value.trim();
  if (!text) return;

  // Optimistic UI
  appendMessage("Me", text);
  input.value = "";

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
    appendMessage("Error", text);
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
