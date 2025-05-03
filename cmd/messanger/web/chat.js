// web/chat.js

// Only the signaling port needs to come from Go
const { wsPort } = window.config;
const host       = location.hostname;

let room, peerId;

// 1 Open the signaling WebSocket for SDP & ICE
let sigWs;
function setupSignaling() {
  sigWs = new WebSocket(`ws://${host}:${wsPort}/signal?room=${room}&peer_id=${peerId}`);
  sigWs.addEventListener("open", () => console.log("🔑 Signaling WS open"));
  sigWs.addEventListener("message", async evt => {
    const msg = JSON.parse(evt.data);
    // Ignore our own signals
    if (msg.peer_id === peerId) return;

    if (msg.type === "offer") {
      await pc.setRemoteDescription(new RTCSessionDescription({ type: "offer", sdp: msg.payload }));
      const answer = await pc.createAnswer();
      await pc.setLocalDescription(answer);
      sendSignal("answer", answer.sdp);

    } else if (msg.type === "answer") {
      await pc.setRemoteDescription(new RTCSessionDescription({ type: "answer", sdp: msg.payload }));

    } else if (msg.type === "ice") {
      try {
        await pc.addIceCandidate(JSON.parse(msg.payload));
      } catch (e) {
        console.warn("Error adding ICE candidate:", e);
      }
    }
  });
}

function sendSignal(type, payload) {
  sigWs.send(JSON.stringify({ peer_id: peerId, type, payload }));
}

// 2 Create the RTCPeerConnection with STUN
const pc = new RTCPeerConnection({
  iceServers: [
    { urls: "stun:stun.l.google.com:19302" },
    // Add TURN here if needed
  ]
});

// Relay local ICE candidates
pc.onicecandidate = ({ candidate }) => {
  if (candidate) {
    sendSignal("ice", JSON.stringify(candidate));
  }
};

// When a remote DataChannel arrives
pc.ondatachannel = ({ channel }) => {
  setupDataChannel(channel);
};

// 3 On page load, wire up everything
window.addEventListener("load", () => {
  peerId = crypto.randomUUID();

  // Grab room from URL or prompt
  const params = new URLSearchParams(location.search);
  room = params.get("room") || prompt("Room name?");
  if (!room) return alert("Room is required");

  document.getElementById("room-info").textContent = `Room: ${room}`;
  document.getElementById("create-room-btn").classList.add("hidden");

  setupSignaling();

  // Create our outgoing data channel
  const channel = pc.createDataChannel("chat");
  setupDataChannel(channel);

  // Caller creates the offer
  pc.createOffer().then(offer => pc.setLocalDescription(offer))
                 .then(() => sendSignal("offer", pc.localDescription.sdp));
});

// 4 Setup a DataChannel for chat
function setupDataChannel(channel) {
  channel.onopen = () => {
    console.log("📡 DataChannel open");
    document.getElementById("send-btn").disabled = false;
  };
  channel.onmessage = ({ data }) => {
    appendMessage("Peer", data);
  };

  document.getElementById("send-btn")
          .addEventListener("click", () => {
    const input = document.getElementById("msg-input");
    const text  = input.value.trim();
    if (!text) return;
    channel.send(text);
    appendMessage("Me", text);
    input.value = "";
  });
}

// 5 Render a chat line
function appendMessage(from, msg) {
  const container = document.getElementById("messages");
  const div = document.createElement("div");
  div.textContent = `${from}: ${msg}`;
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
}
