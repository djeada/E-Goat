# E-Goat

E-Goat is a lightweight P2P messaging application packaged as a single static Go binary. It uses a minimal WebSocket “signaling” service to bootstrap WebRTC peer connections, and a simple HTTP+polling REST API for text chat fallback. All message history is kept in a local SQLite database (`chat.db`), so you can back up or migrate your chat log at any time.
![e_goat](https://github.com/user-attachments/assets/9ac71bdd-1fe3-41b9-89c0-21de1e140ced)

## Features

- **Pure P2P Mesh**  
  - WebRTC DataChannels (via Pion) connect every participant directly in a full mesh.  
  - No central relay of your chat or file data—only SDP/ICE goes through the signaling server.

- **HTTP-Polling Chat Fallback**  
  - For cases where you can’t traverse NATs, the UI falls back to a REST API:  
    - `GET  /history?room=<name>&since=<ts>`  
    - `POST /send`  
  - `chat.js` polls `/history` once per second and posts new messages to `/send`.

- **Local Storage**  
  - All messages (signaling, text, file chunks) are stored on-disk in SQLite.  
  - Simply copy `chat.db` to back up or import on another machine.

- **Single-Binary Distribution**  
  - Built in Go, cross-compilable for Windows, macOS, and Linux.  
  - No dependencies beyond a web browser.

---

## NAT Traversal & Port Forwarding

By default the HTTP server (`:8080`) and signaling WS (`:9000`) listen on all interfaces, but most home networks sit behind NAT. To allow external peers to reach you without WebRTC STUN/TURN:

1. **Router Port-Forwarding**  
   - Forward external TCP port **8080** → your machine’s LAN IP on port **8080**  
   - (Optional for signaling fallback) Forward TCP port **9000** → your LAN IP on **9000**

2. **Linux Firewall**  
   - **UFW**  
     ```bash
     sudo ufw allow 8080/tcp
     sudo ufw allow 9000/tcp
     sudo ufw reload
     ```
   - **firewalld**  
     ```bash
     sudo firewall-cmd --add-port=8080/tcp --permanent
     sudo firewall-cmd --add-port=9000/tcp --permanent
     sudo firewall-cmd --reload
     ```
   - **iptables**  
     ```bash
     sudo iptables -A INPUT -p tcp --dport 8080 -j ACCEPT
     sudo iptables -A INPUT -p tcp --dport 9000 -j ACCEPT
     sudo apt install iptables-persistent
     sudo netfilter-persistent save
     ```

Once forwarded and allowed, external links like  

```
http\://YOUR\_WAN\_IP:8080/?room=myroom\&peer\_id=XYZ
```

will load the UI and enable chat.

## Architectural Overview

E-Goat employs a hybrid design: a minimal **signaling** component to bootstrap peer connections, plus a **polling-based** fallback for text chat when direct P2P links can’t be established without NAT configuration. All persistent state lives locally in SQLite.

### 1. Signaling Phase

A tiny WebSocket service (built into the binary) simply relays SDP offers/answers and ICE candidates.  No chat or file data ever passes through it.

```text
Client A (Browser)           Signaling Server           Client B (Browser)
+------------------+         +------------------+       +------------------+
|                  |   ----> |                  | <---- |                  |
|  WS: offer,      |         |  broadcast to    |       |  WS: offer/ICE   |
|      answer, ICE |         |  room members    |       |                  |
+------------------+         +------------------+       +------------------+
```

* **Purpose**: Enable browsers to discover each other’s IP/port via STUN/ICE, and exchange SDP metadata.
* **Stateless**: Only handles JSON envelopes `{ peer_id, type, payload }`.

### 2. P2P Mesh via WebRTC

Once both ends have exchanged SDP and ICE, they open native WebRTC DataChannels for direct, encrypted, low-latency transport:

```text
       Peer A
       /    \
      /      \
 Peer B ———— Peer C
      \      /
       \    /
       Peer D
```

* **DataChannels** carry both text messages and binary file chunks.
* **Full Mesh**: Each of N peers connects directly to every other peer (N·(N-1)/2 links). Ideal for small groups.
* **NAT Traversal**: Uses STUN servers to punch through most home NATs; TURN can be added if needed.

### 3. HTTP-Polling Chat Fallback

If you don’t want to configure port-forwarding or your NAT/STUN setup fails, E-Goat falls back to a simple REST API:

```text
Browser                        E-Goat HTTP Server (Go)
+----------------+             +-------------------------+
|                | GET /history?room=X&since=T        |
| poll every 1s  | ----------------------------------> |
|                |                                     |
|                | <---------------------------------- |
|                |  [ {peer_id, text, timestamp}, … ]   |
+----------------+                                     |
| POST /send     | --{room,peer_id,text}-------------->|
|                |                                     |
|                | <----{ timestamp }------------------|
+----------------+                                     |
```

* **`GET /history`** returns all new text messages since the last poll.
* **`POST /send`** appends a message and returns its timestamp.
* **Pros**: Works over plain HTTP on ports you forward.
* **Cons**: Higher latency (1 s polling), extra load on the server.

---

### 4. Storage & UI Layers

```text
+---------------------------------------------------+
| E-Goat Binary                                     |
|  ┌───────────────────┐   ┌─────────────────────┐  |
|  │ HTTP Server       │   │ Signaling WS Server │  |
|  │ - Serves UI       │   │ - SDP/ICE relay     │  |
|  │ - REST endpoints  │   └─────────────────────┘  |
|  └───────────────────┘                             |
|  ┌───────────────────┐   ┌─────────────────────┐  |
|  │ Pion WebRTC Module│   │ SQLite Storage      │  |
|  │ (optional DataCh) │   │ - `messages` table  │  |
|  └───────────────────┘   └─────────────────────┘  |
+---------------------------------------------------+
               ↓
       Web Browser UI (index.html, chat.js)
```

* **Local HTTP Server** (`:8080`):

  * Serves `index.html`, `styles.css`, and `chat.js`.
  * Exposes `/history` and `/send` for polling chat.

* **Signaling WS** (`:9000`):

  * Exposes `/signal` for WebRTC bootstrapping.

* **Pion WebRTC** (`peer.go`):

  * Manages `RTCPeerConnection` and `DataChannel` mesh logic.

* **SQLite** (`sqlite.go`):

  * Tables for `peers` and `messages` (text & file chunks).
  * `SaveMessage` persists every chat and signaling envelope.

* **Front-End (`chat.js`)**:

  * Auto-joins via `?room=…&peer_id=…`.
  * Displays your external IP and generates a deep-link invite.
  * Implements polling fallback: fetch `/history` every second and post to `/send`.

---

This architecture lets you choose:

* **Pure P2P** with WebRTC for minimal latency and no router config (when STUN/TURN works).
* **HTTP-Polling** when you prefer manual port-forwarding or can’t set up STUN/TURN, trading off extra hops for simplicity.



## Technologies & Dependencies

* **Go**

  * Core language for single-binary builds.
  * Standard library: `net/http`, `database/sql`, `embed`, `flag`.

* **WebRTC**

  * `github.com/pion/webrtc/v3`: Peer connection and DataChannel implementation in Go.

* **SQLite**

  * `github.com/mattn/go-sqlite3`: Embedded on-disk database.

* **Frontend**

  * Vanilla HTML, CSS, and JavaScript (WebRTC and WebSocket APIs).


## REST & Signaling APIs

### HTTP Endpoints (port `8080`)
- `GET  /`  
  Serves the chat UI (`index.html`, `styles.css`, `chat.js`).

- `GET  /static/*`  
  Serves front-end assets.

- `GET  /history?room=<name>&since=<ts>`  
  Returns JSON array of all text messages in `room` with `timestamp > since`.

```json
[
{ "peer_id": "A1B2", "text": "Hello", "timestamp": 1620000000 },
…
]
```

* `POST /send`
  Accepts `{ room, peer_id, text }` JSON and saves a new message.
  Returns `{ timestamp }` on success.

### WebSocket Signaling (port `9000`)

* `ws://<host>:9000/signal?room=<name>&peer_id=<id>`
  Simple hub that relays SDP offers, answers, and ICE candidates between all clients in the same `room`.
  Signal messages are JSON:

  ```json
  { "peer_id":"A1B2", "type":"offer",   "payload":"<SDP>" }
  { "peer_id":"C3D4", "type":"ice",     "payload":"<ICE JSON>" }
  { "peer_id":"A1B2", "type":"answer",  "payload":"<SDP>" }
  ```

## Front-End (`web/chat.js`)

* On load, generates a `peer_id` and optionally auto-joins if `?room=…&peer_id=…` are in the URL.
* **`joinRoom()`**

  * Hides the init UI, shows chat UI, displays an HTTP deep-link invite.
  * Starts polling `/history?since=<lastTimestamp>` every second.
* **`pollHistory()`**

  * Fetches new messages and appends them to the chat view.
* **`sendMessage()`**

  * Posts `{ room, peer_id, text }` to `/send`.
  * Optimistically renders “Me: …” and marks errors if the POST fails.


## Building & Running

```bash
# From project root
go build -o e-goat ./cmd/messanger

# Run with defaults:
./e-goat \
  -http-port=8080 \
  -ws-port=9000 \
  -db=chat.db
```

Open [http://localhost:8080](http://localhost:8080), create or join a room, copy the invite link, and share it. If your network is NAT’d without STUN/TURN, configure port-forwarding and firewall rules as above.




## Project Layout

```
E-Goat/
├── cmd/
│   └── messanger/
│       ├── main.go
│       └── web/
│           ├── index.html       # Chat UI
│           ├── chat.js          # Polling-based front-end logic
│           └── styles.css       # Minimal styling
├── internal/
│   ├── signaling/
│   │   └── server.go            # WebSocket signaling hub
│   ├── storage/
│   │   └── sqlite.go            # SQLite init & SaveMessage
│   └── webrtc/
│       └── peer.go              # Pion WebRTC mesh logic
├── go.mod
├── go.sum
├── LICENSE
└── README.md                    # This file
```

## Contributing

We welcome pull requests. For significant changes, kindly open an issue first for discussion. Please ensure to update the tests as necessary with your changes.

## License

E-Goat is licensed under the [MIT License](https://choosealicense.com/licenses/mit/).
