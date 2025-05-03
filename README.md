# E-Goat

E-Goat is a lightweight, fully peer-to-peer (P2P) messaging application that enables text and file transfers among any number of participants—no central server required. Packaged as a single static binary for Windows, macOS, and Linux, E-Goat is easy to distribute and run.

![e_goat](https://github.com/user-attachments/assets/9ac71bdd-1fe3-41b9-89c0-21de1e140ced)

## Features

* **True Peer-to-Peer**: Direct mesh network via WebRTC DataChannels; every participant connects to every other participant.
* **Group Chat**: Support for 2+ participants per room without any central relay.
* **Text & File Exchange**: Instant messaging and chunked file transfer over P2P links.
* **Local Storage**: SQLite-based on-disk history (`chat.db`) for backup and migration.
* **Single-Binary Distribution**: Built in Go, no runtime dependencies beyond a web browser.
* **Minimal External Libraries**: Go standard library, Pion WebRTC, and SQLite driver only.

## Architectural Overview

E-Goat’s design centers on a pure P2P mesh using WebRTC. A lightweight signaling step is the only centralized component, used solely to bootstrap peer connections—once established, all data flows directly between clients.

### 1. Signaling Phase

```text
+------------------+          +------------------+
| Client A         |          | Client B         |
| (Browser + HTTP) |          | (Browser + HTTP) |
+--------+---------+          +---------+--------+
         |                             |
         | WebSocket Signaling (offer/answer + ICE candidates)
         |                             |
+--------v---------+          +---------v--------+
| Signaling Server |          |                  |
+------------------+          +------------------+
```

* **Signaling Server**: A minimal WebSocket service (embedded in the binary) that relays SDP offers, answers, and ICE candidates between peers. No media or chat data passes through.

### 2. Mesh Topology

Once signaling completes, each peer opens direct WebRTC DataChannels to every other peer in the room, forming a full mesh.

```text
        Peer A
       /      \
      /        \
  Peer B ---- Peer C
      \        /
       \      /
        Peer D
```

* **DataChannels**: Carry both text messages and file chunks (e.g., Base64 or binary frames).
* **Scalability**: For N participants, N\*(N-1)/2 direct connections; optimal for small friend groups.

### 3. Storage & UI

```text
+------------------------+
| E-Goat Binary          |
| ┌────────────────────┐ |
| | HTTP Server        | |
| | - Serves UI assets | |
| | - Offers REST API  | |
| └────────────────────┘ |
|                        |
| ┌────────────────────┐ |
| | Signaling WS Server| |
| └────────────────────┘ |
|                        |
| ┌────────────────────┐ |
| | Pion WebRTC Module | |
| └────────────────────┘ |
|                        |
| ┌────────────────────┐ |
| | SQLite Storage     | |
| | (`chat.db`)        | |
| └────────────────────┘ |
+------------------------+
          |
          v
  Web Browser UI
  (index.html, chat.js)
```

* **Local HTTP Server**: Hosts the front-end (HTML/CSS/JS) at `http://localhost:PORT` and the WebSocket endpoint for signaling.
* **SQLite Layer**: Stores messages and file metadata. Users can copy `chat.db` to back up or migrate their history.
* **Frontend**: Vanilla JavaScript handles UI rendering, WebRTC DataChannel management, and local persistence calls.

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


## Project Layout

```
E-Goat/
├── cmd/messenger/
│   └── main.go           # Entrypoint: HTTP + WS servers, WebRTC init
├── internal/
│   ├── signaling/
│   │   └── server.go     # WebSocket signaling handlers
│   ├── webrtc/
│   │   └── peer.go       # Pion WebRTC mesh logic
│   ├── storage/
│   │   └── sqlite.go     # DB setup and CRUD operations
│   └── ui/
│       ├── assets.go     # //embed HTML/CSS/JS
│       └── handler.go    # HTTP handlers for UI and REST
├── web/
│   ├── index.html        # Chat interface
│   ├── chat.js           # WebRTC + signaling logic
│   └── styles.css        # Simple styling
├── go.mod                # Module definitions
└── README.md             # This file
```

## Contributing

We welcome pull requests. For significant changes, kindly open an issue first for discussion. Please ensure to update the tests as necessary with your changes.

## License

E-Goat is licensed under the [MIT License](https://choosealicense.com/licenses/mit/).
