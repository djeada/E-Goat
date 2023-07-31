"""
A peer-to-peer client used to connect to other nodes in the network.

This client uses a low-level network client to handle networking, and adds message handling on top.
"""

from threading import Thread
from typing import Any, Dict

from src.lib.p2p.client.network_client import NetworkClient
from src.lib.p2p.utils.message_protocol import MessageProtocol


class P2PClient:
    """
    The high-level P2P client.

    Attributes:
        node: The node this client belongs to.
        message_protocol: The protocol for encoding and decoding messages.
    """

    def __init__(self, node: Any):
        """
        Initialize a new P2PClient instance.

        Args:
            node: The node this client belongs to.
        """
        self.node = node
        self.message_protocol: MessageProtocol = MessageProtocol()

    def start(self) -> None:
        """Start the client."""
        pass

    def stop(self) -> None:
        """Stop the client."""
        pass

    def send_message(self, host: str, port: int, message_type: str, data: Any) -> None:
        """
        Send a message to another node.

        Args:
            host: The host address of the node to send to.
            port: The port number of the node to send to.
            message_type: The type of the message to send.
            data: The data of the message to send.
        """
        message: Dict[str, Any] = {
            "type": message_type,
            "data": data,
        }
        encoded_message: bytes = self.message_protocol.encode_message(message)
        Thread(target=self.connect_and_send, args=(host, port, encoded_message)).start()

    def connect_and_send(self, host: str, port: int, encoded_message: bytes) -> None:
        """
        Connect to another node and send it a message.

        Args:
            host: The host address of the node to connect to.
            port: The port number of the node to connect to.
            encoded_message: The encoded message to send.
        """
        network_client: NetworkClient = NetworkClient()
        try:
            network_client.connect(host, port)
        except ConnectionRefusedError:
            self.node.remove_peer(host, port)
            return
        try:
            network_client.send(encoded_message)
        finally:
            network_client.close()
