"""
P2PServer is the high-level server. It uses NetworkServer to accept and handle connections, and then further processes messages as needed by your application in handle_message method.
"""

import queue
from typing import Any, Dict, Optional

from src.lib.p2p.server.network_server import NetworkServer
from src.lib.p2p.utils.message_protocol import MessageProtocol


class P2PServer:
    """
    The high-level P2P server.

    Attributes:
        node: The node this server belongs to.
        host: The server's host address.
        port: The port that the server listens on.
        network_server: The low-level network server that this server uses.
        running: A flag indicating whether this server is running.
        message_protocol: The protocol for encoding and decoding messages.
        chat_queue: A queue storing chat messages.
    """

    def __init__(self, node: Any, host: str, port: int):
        """
        Initialize a new P2PServer instance.

        Args:
            node: The node this server belongs to.
            host: The server's host address.
            port: The port that the server listens on.
        """
        self.node = node
        self.host = host
        self.port = port
        self.network_server = NetworkServer(host, port, self.handle_message)
        self.running: bool = False
        self.message_protocol: MessageProtocol = MessageProtocol()
        self.chat_queue: queue.Queue = queue.Queue()  # Queue for storing chat messages

    def start(self) -> None:
        """Start the server."""
        self.running = True
        self.network_server.start()

    def stop(self) -> None:
        """Stop the server."""
        self.running = False
        self.network_server.stop()

    def handle_message(self, data: bytes) -> None:
        """
        Handle an incoming message.

        Args:
            data: The raw message data to handle.
        """
        message: Dict[str, Any] = self.message_protocol.decode_message(data)
        message_type: str = message["type"]
        if message_type == "chat":
            self.chat_queue.put(message["data"])  # Add chat messages to the queue
        elif message_type == "request_peers":
            self.node.broadcast_message(
                self.node.get_serializable_peers(),
                message_type="update_peers",
            )
        elif message_type == "update_peers":
            self.node.update_peers(message["data"])
        else:
            print(f"Unknown message type: {message_type}")

    def read_next_message(self) -> Optional[str]:
        """
        Read the next message from the chat queue.

        Returns:
            The next message from the chat queue, if one exists.
        """
        return self.chat_queue.get()
