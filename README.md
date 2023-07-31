# E-Goat

E-Goat is a powerful real-time chat application, implemented with a strong focus on the peer-to-peer (P2P) network architecture. The application utilizes the TCP protocol, thereby ensuring reliable data transmission. The decentralized design of E-Goat makes it resilient to network failures and eliminates dependency on central servers.

## Core Features

- **Peer-to-Peer Communication**: E-Goat facilitates direct, server-less communication between devices.
- **Security**: Our app uses end-to-end encryption, ensuring the privacy and security of all communications.
- **Reliability**: Thanks to the decentralized architecture, E-Goat is highly resilient to network failures.
- **Cross-Platform Compatibility**: E-Goat is designed to work across different operating systems and device types.
- **Offline Messaging**: Messages are stored offline and are delivered once the recipient comes online.
- **File Sharing**: You can share files directly with your peers without uploading them to a server first.
- **Chat History**: E-Goat stores your chat history locally, allowing you to access past conversations.

## Prerequisites

Before starting the installation, ensure that you have the following installed on your machine:

- Git
- Python 3.9 or higher
- pip (Python package installer)

## Installation

To install E-Goat, follow the steps outlined below:

1. Clone the repository: This will download a copy of the E-Goat source code onto your local machine. Open a terminal window and run the following command:

```bash
git clone https://github.com/djeada/E-Goat.git
```

2. Navigate to the project directory: Change your current working directory to the cloned E-Goat directory:

```bash
cd E-Goat
```

3. Install the required packages: E-Goat has a number of Python package dependencies. These can be installed by running the following command:

```bash
pip install -r requirements.txt
```

Please note that it is recommended to use a virtual environment to avoid conflicts with other Python projects.

## Usage

Once you have successfully installed E-Goat, you can start using it by following the steps outlined below:

1. Run the application: Open a terminal window, ensure you are in the E-Goat directory, and run the following command:

```bash
python -m src.main localhost 3333
```

Replace 'localhost' and '3333' with the host address and port number you wish to use.

2. Follow the prompts: Once the application starts, you will be guided through the process of connecting with a peer and starting a chat. Simply follow the prompts on the screen to begin chatting.

## System Desing

A visual representation of the structure is shown below:

```
[C/S]<--->[C/S]<--->[C/S]
  ^         ^         ^ 
  |         |         |
  v         v         v
[C/S]<--->[C/S]<--->[C/S]
  ^         ^         ^ 
  |         |         |
  v         v         v
[C/S]<--->[C/S]<--->[C/S]
```

In this diagram, each `[C/S]` symbolizes a node in the network, with the two-way arrows (`<--->`) indicating a bi-directional communication link. In this model, all nodes are fully connected, allowing each to communicate directly with every other node. Such an arrangement is referred to as a fully connected or complete network. While it's the most resilient network type, it's also the most complex due to the high number of links required. For a network with N nodes, there are N*(N-1)/2 links.

Components:

- P2PNode: A P2PNode is used for each node in the network. It combines the functionalities of the P2PServer and the P2PClient.
- P2PServer: Every node operates a server instance in a separate thread, ready to receive connections from other peers. The server can decode and process messages according to the established protocol.
- P2PClient: Each node uses a client instance to connect with other peers and to dispatch messages. Messages are encoded in line with the protocol and transmitted over the established connection.
- NetworkClient: This is a low-level wrapper for a socket that connects to a server and dispatches a message. After each message transmission, the client socket is closed.
- NetworkServer: This is a low-level wrapper for a server socket. It constantly listens for incoming connections, forwarding the client socket and the received data to a predefined handler function.

The system architecture is designed to manage multi-threading. Each outgoing connection from a client to a server operates in its own thread. This enables asynchronous communication, where a node can establish several connections and communicate with various peers simultaneously.

Additionally, the system incorporates error handling to manage issues such as failed connections. The design of the system prioritizes modularity, ensuring a clear delineation of roles among the classes.

## External Communication

Communicating with devices outside your Local Area Network (LAN) can be restricted due to residential network ISP limitations. To effectively use IPv4, your router must have an IPv4 WAN address. If it's not available, it might be because your ISP is implementing NAT, which prevents you from forwarding a port and using IPv4. For IPv6, you should use the Global IPv6 address assigned to your server, not the WAN address.

## References

- [Open Computer Science Fundamentals](https://w3.cs.jmu.edu/kirkpams/OpenCSF/Books/csf/html/)
- [MIT 6.005: Software Construction](http://web.mit.edu/6.005/www/fa15/classes/21-sockets-networking/)
- [Beej's Guide to Network Programming](https://beej.us/guide/bgnet/)
- [Writing a Web Server in Python](https://iximiuz.com/en/posts/writing-web-server-in-python-sockets/)

## Contributing

We welcome pull requests. For significant changes, kindly open an issue first for discussion. Please ensure to update the tests as necessary with your changes.

## License

E-Goat is licensed under the [MIT License](https://choosealicense.com/licenses/mit/).
