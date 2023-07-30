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

## Installation

To get E-Goat up and running, you need to:

1. Clone the repository:

```bash
git clone https://github.com/your-username/P2PCommunicator.git
```

2. Navigate to the project directory

```bash
cd P2PCommunicator
```

3. Install the required packages

```bash
pip install -r requirements.txt
```

## Usage

To start using E-Goat, run the application:

```bash
python3 main.py
```

Then, simply follow the prompts to connect with a peer and start chatting.


## System Desing

The following diagram depicts the structure of our application:

```
+-------------+            +-------------+
|   P2PNode   |            |   P2PNode   |
|   Server    |<---->      |   Server    |
|   (host1)   |   connect  |   (host2)   |
|   Port:5000 |   <---->   |   Port:5001 |
+------+------+            +------+------+
       |                            |
       |                            |
       v                            v
+------+-------+            +-------+------+
| P2PNodeRunner|            | P2PNodeRunner|
|   User       |            |   User       |
|  Interface   |            |  Interface   |
| (localhost)  |            | (localhost)  |
|   Port:5000  |            |   Port:5001  |
+--------------+            +--------------+
```

## External Communication

Communicating with devices outside your Local Area Network (LAN) can be restricted due to residential network ISP limitations. To effectively use IPv4, your router must have an IPv4 WAN address. If it's not available, it might be because your ISP is implementing NAT, which prevents you from forwarding a port and using IPv4. For IPv6, you should use the Global IPv6 address assigned to your server, not the WAN address.

## The Necessity of Threads in Networking

Threads are integral in networking due to the following reasons:

- They permit parallel processing, allowing multiple network connections to be handled simultaneously.
- Complex network tasks can be segmented and managed in an efficient, organized manner.
- They offer responsiveness and scalability in server applications, enabling the handling of numerous incoming requests at once.
- Concurrent running of background tasks, such as sending keep-alive signals, without impacting the main application is possible.

One server socket per node, and N client sockets: In this setup, each node maintains a server socket to listen for incoming connections and a client socket for every other node in the network to initiate connections. This could work but it can be resource-intensive and might not scale well. As the number of nodes increases, the number of sockets also increases quadratically, leading to more complexity in managing connections.

Socket-per-thread approach: Here, each socket connection (whether client or server) is managed by a separate thread. This approach can be effective at handling multiple simultaneous connections as it allows for concurrent processing of each connection. However, it can also become resource-intensive as the number of threads increases, as each thread consumes system resources (e.g., memory for stack space). There's also the added complexity of thread management and synchronization to prevent data races and inconsistencies.


## Areas for Improvement and Expansion

E-Goat currently lacks:

- Robust Error Handling: Given the susceptibility of network programming to errors, a solid error handling mechanism is crucial.
- Non-blocking I/O: The current version uses blocking I/O, hindering the simultaneous execution of other tasks. 
- Handling Multiple Connections: The present system can manage multiple incoming connections but only one peer connection at a time.
- Encryption: To ensure secure communication, messages and files need to be encrypted.
- Authentication: A method for authenticating peers should be implemented.
- File Chunking: Large files should be broken into smaller chunks for more efficient transmission.
- File Reconstruction: A mechanism for reassembling file chunks into the original file is necessary.
- Graphical User Interface (GUI): A GUI would improve user interaction, replacing the current console-only output.

## References

- [Open Computer Science Fundamentals](https://w3.cs.jmu.edu/kirkpams/OpenCSF/Books/csf/html/)
- [MIT 6.005: Software Construction](http://web.mit.edu/6.005/www/fa15/classes/21-sockets-networking/)
- [Beej's Guide to Network Programming](https://beej.us/guide/bgnet/)
- [Writing a Web Server in Python](https://iximiuz.com/en/posts/writing-web-server-in-python-sockets/)

## Contributing

We welcome pull requests. For significant changes, kindly open an issue first for discussion. Please ensure to update the tests as necessary with your changes.

## License

E-Goat is licensed under the [MIT License](https://choosealicense.com/licenses/mit/).
