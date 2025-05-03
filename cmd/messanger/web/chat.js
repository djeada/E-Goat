// Grab the ports, and the host from the page context
const { wsPort, httpPort } = window.config;
const host = location.hostname;

let room, peerId, myIP;
let sigWs, chatWs;

// On load…
window.addEventListener("load", () => {
  peerId = crypto.randomUUID();
  fetchIP();
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

  // Build invitation string
  const invite = `Connect to IP: ${myIP}:${wsPort}, Room: ${room}`;
  const invEl = document.getElementById("invite-text");
  invEl.value = invite;
  document.getElementById("invitation")
          .classList.remove("hidden");

  // Swap views
  document.getElementById("init").classList.add("hidden");
  document.getElementById("chat").classList.remove("hidden");
  document.getElementById("room-info")
          .textContent = `Room: ${room}`;

  // Open websockets
  openSignalingWS();
  openChatWS();
}

// 3. Signaling WS for WebRTC handshake
function openSignalingWS() {
  sigWs = new WebSocket(
    `ws://${host}:${wsPort}/signal?room=${room}&peer_id=${peerId}`
  );
  sigWs.addEventListener("open", () => {
    console.log("🔑 Signaling WS open");
  });
  sigWs.addEventListener("message", evt => {
    console.debug("⏳ Signal:", evt.data);
    // Server-side Pion peers handle these under-the-hood
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
    appendMessage(msg.peer_id, msg.text);
  });
}

// 5. Sending a message
function sendMessage() {
  const input = document.getElementById("msg-input");
  const text  = input.value.trim();
  if (!text || !chatWs || chatWs.readyState !== 1) return;
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
