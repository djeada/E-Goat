import socket
import threading
import json
import time
import queue
import logging
from typing import List, Tuple, Optional, Dict
from dataclasses import dataclass
from .client import P2PClient
from .server import P2PServer


@dataclass(frozen=True)
class Peer:
    host: str
    port: int


class P2PNode:
    def __init__(self, host: str, port: int):
        self.logger = logging.getLogger(__name__)
        self.host = host
        self.port = port
        self.server = P2PServer(self, host, port)  # own server
        self.client = P2PClient(self)  # for client connections to other servers
        self.peers = set()

    def start(self):
        self.server.start()
        self.client.start()

    def stop(self):
        self.server.stop()
        self.client.stop()

    def get_serializable_peers(self) -> List[Tuple[str, int]]:
        return [(peer.host, peer.port) for peer in self.peers] + [
            (self.host, self.port)
        ]

    def add_peer(self, peer: Peer):
        if peer in self.peers:
            print(f"Peer {peer} already added!")
            return

        self.peers.add(peer)
        self.broadcast_message(
            self.get_serializable_peers(), message_type="update_peers"
        )

    def remove_peer(self, host, port):
        peer = Peer(host, port)
        if peer not in self.peers:
            print(f"Peer {peer} already removed!")
            return

        self.peers.remove(peer)
        self.broadcast_message(
            self.get_serializable_peers(), message_type="update_peers"
        )

    def connect_to_peer(self, host: str, port: int) -> bool:
        peer = Peer(host, port)

        if peer in self.peers:
            print(f"Peer {peer} already added!")
            return False

        def _exchange_peers_info():
            self.client.send_message(peer.host, peer.port, "request_peers", None)
            self.client.send_message(
                peer.host, peer.port, "update_peers", self.get_serializable_peers()
            )

        print(f"Connected to new peer {host}:{port}")
        self.add_peer(peer)
        _exchange_peers_info()
        return True

    def update_peers(self, peers: List[Tuple[str, int]]):
        print(f"\nNew peers to update {peers}")
        for host, port in peers:
            if (host, port) == (self.host, self.port) or Peer(host, port) in self.peers:
                continue
            if self.connect_to_peer(host, port):
                print(f"Discovered and connected to new peer: {host}:{port}")
            else:
                print(f"Could not connect to peer: {host}:{port}")

    def read_next_message(self) -> str:
        return self.server.read_next_message()

    def broadcast_message(self, message: str, message_type: Optional[str] = "chat"):
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
