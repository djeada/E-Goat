import socket
import threading
import json
import time
import queue
from typing import List, Tuple
from dataclasses import dataclass
from .message_protocol import MessageProtocol

import socket
import json
import threading


class NetworkClient:
    def __init__(self):
        self.client_socket = None

    def connect(self, host, port):
        self.client_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.client_socket.connect((host, port))

    def send(self, message):
        self.client_socket.sendall(message)

    def close(self):
        if self.client_socket is not None:
            self.client_socket.close()
            self.client_socket = None


class P2PClient:
    def __init__(self, node):
        self.node = node
        self.message_protocol = MessageProtocol()

    def start(self):
        pass

    def stop(self):
        pass

    def send_message(self, host: str, port: int, message_type, data):
        message = {
            "type": message_type,
            "data": data,
        }
        encoded_message = self.message_protocol.encode_message(message)
        threading.Thread(
            target=self.connect_and_send, args=(host, port, encoded_message)
        ).start()

    def connect_and_send(self, host, port, encoded_message):
        network_client = NetworkClient()
        try:
            network_client.connect(host, port)
        except ConnectionRefusedError:
            self.node.remove_peer(host, port)
            return
        try:
            network_client.send(encoded_message)
        finally:
            network_client.close()
