// web/chat.js

// Grab config injected by indexHandler
const { wsPort, httpPort } = window.config;

// Prompt (or read) room and peer ID
const room   = prompt("Room name:");
const peerId = prompt("Your peer ID:") || crypto.randomUUID();

// -- 1️⃣ Signaling WebSocket (for WebRTC handshake) --
const sigWs = new WebSocket(
  `ws://${location.hostname}:${wsPort}/signal?room=${room}&peer_id=${peerId}`
);

sigWs.addEventListener("open", () => {
  console.log("🔑 Signaling connected");
});

sigWs.addEventListener("message", async (evt) => {
  // Forward any incoming signal blobs to the server-side Pion Peer automatically—
  // your Go signaling layer will route offers/answers/ICE to the other peers.
  console.debug("⏳ Signal received:", evt.data);
  // (Nothing else to do here: server.HandleSignal is called automatically)
});

// -- 2️⃣ Chat WebSocket (for text chat) --
// You need to add a Go handler at `/chat` that upgrades to WS, registers the client 
// similarly to signaling, and relays JSON `{ peer_id, text }` messages over all
// active WebRTC DataChannels.
const chatWs = new WebSocket(
  `ws://${location.hostname}:${httpPort}/chat?room=${room}&peer_id=${peerId}`
);

chatWs.addEventListener("open", () => {
  console.log("💬 Chat connected");
});

chatWs.addEventListener("message", (evt) => {
  // Expect JSON: { peer_id: string, text: string }
  const msg = JSON.parse(evt.data);
  addMessage(msg.peer_id, msg.text);
});

// Send button wiring
document.getElementById("sendBtn").addEventListener("click", () => {
  const input = document.getElementById("msgInput");
  const text  = input.value.trim();
  if (!text) return;
  // Send over the chat WS
  chatWs.send(JSON.stringify({ peer_id: peerId, text }));
  // Optimistically display your message
  addMessage("Me", text);
  input.value = "";
});

// Utility to append to the message list
function addMessage(from, txt) {
  const container = document.getElementById("messages");
  const line = document.createElement("div");
  line.textContent = `${from}: ${txt}`;
  container.appendChild(line);
  container.scrollTop = container.scrollHeight;
}
