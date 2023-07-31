"""
A peer-to-peer node class, which serves as the main entry point for a P2P application.

A P2P node is a member of a peer-to-peer network, and can act as both a client and a server.
"""

import logging
from typing import List, Tuple, Optional, Set
from dataclasses import dataclass

from src.lib.p2p.client.p2p_client import P2PClient
from src.lib.p2p.server.p2p_server import P2PServer


@dataclass(frozen=True)
class Peer:
    """
    A class representing a peer in a peer-to-peer network.

    Attributes:
        host: The host address of the peer.
        port: The port number of the peer.
    """
    host: str
    port: int


class P2PNode:
    """
    A node in a peer-to-peer network.

    Attributes:
        host: The host address of this node.
        port: The port number of this node.
        server: The server instance for this node.
        client: The client instance for this node.
        peers: The set of peers this node is connected to.
    """
    def __init__(self, host: str, port: int):
        self.logger: logging.Logger = logging.getLogger(__name__)
        self.host: str = host
        self.port: int = port
        self.server: P2PServer = P2PServer(self, host, port)  # own server
        self.client: P2PClient = P2PClient(self)  # for client connections to other servers
        self.peers: Set[Peer] = set()

    def start(self) -> None:
        """Start the node."""
        self.server.start()
        self.client.start()

    def stop(self) -> None:
        """Stop the node."""
        self.server.stop()
        self.client.stop()

    def get_serializable_peers(self) -> List[Tuple[str, int]]:
        """Get a list of the peers, serialized as tuples of host address and port number."""
        return [(peer.host, peer.port) for peer in self.peers] + [(self.host, self.port)]

    def add_peer(self, peer: Peer) -> None:
        """
        Add a peer to the set of peers.

        If the peer is already in the set, does nothing.
        """
        if peer in self.peers:
            print(f"Peer {peer} already added!")
            return

        self.peers.add(peer)
        self.broadcast_message(
            self.get_serializable_peers(), message_type="update_peers"
        )

    def remove_peer(self, host: str, port: int) -> None:
        """
        Remove a peer from the set of peers.

        If the peer is not in the set, does nothing.

        Args:
            host: The host address of the peer.
            port: The port number of the peer.
        """
        peer = Peer(host, port)
        if peer not in self.peers:
            print(f"Peer {peer} already removed!")
            return

        self.peers.remove(peer)
        self.broadcast_message(
            self.get_serializable_peers(), message_type="update_peers"
        )

    def connect_to_peer(self, host: str, port: int) -> bool:
        """
        Connect to a new peer and add it to the set of peers.

        If the peer is already in the set, does nothing and returns False.

        Args:
            host: The host address of the peer.
            port: The port number of the peer.

        Returns:
            True if the connection was successful, False otherwise.
        """
        peer = Peer(host, port)

        if peer in self.peers:
            print(f"Peer {peer} already added!")
            return False

        def _exchange_peers_info():
            self.client.send_message(peer.host, peer.port, "request_peers",
                                     None)
            self.client.send_message(
                peer.host, peer.port, "update_peers",
                self.get_serializable_peers()
            )

        print(f"Connected to new peer {host}:{port}")
        self.add_peer(peer)
        _exchange_peers_info()
        return True

    def update_peers(self, peers: List[Tuple[str, int]]) -> None:
        """
        Update the set of peers based on the provided list.

        If a peer is already in the set or is this node itself, it is ignored.

        Args:
            peers: A list of peers, represented as tuples of host address and port number.
        """
        print(f"\nNew peers to update {peers}")
        for host, port in peers:
            if (host, port) == (self.host, self.port) or Peer(host,
                                                              port) in self.peers:
                continue
            if self.connect_to_peer(host, port):
                print(f"Discovered and connected to new peer: {host}:{port}")
            else:
                print(f"Could not connect to peer: {host}:{port}")

    def read_next_message(self) -> str:
        """
        Read the next message from the server.

        Returns:
            The message string.
        """
        return self.server.read_next_message()

    def broadcast_message(self, message: str, message_type: Optional[str] = "chat") -> None:
        """
        Broadcast a message to all peers.

        If sending the message to a peer fails, that peer is removed from the set of peers.

        Args:
            message: The message string to broadcast.
            message_type: The type of the message. Defaults to "chat".
        """
        print(f"\nBroadcasting to peers: {self.peers}")
        for peer in list(
            self.peers
        ):  # Create a copy of the set to avoid 'set changed size during iteration' error
            try:
                self.client.send_message(peer.host, peer.port, message_type, message)
            except Exception as e:
                print(
                    f"Failed to send message to {peer.host}:{peer.port}. Error: {str(e)}"
                )
                self.peers.remove(peer)
