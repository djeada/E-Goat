"""
NetworkServer is the low-level server that handles networking. It accepts connections and then spawns a new thread to handle each connection, calling a provided handler function with the client socket and the message.

P2PServer is the high-level server. It uses NetworkServer to accept and handle connections, and then further processes messages as needed by your application in handle_message method.
"""


import socket
import threading
import json
import time
import queue
from typing import List, Tuple
from dataclasses import dataclass
from .message_protocol import MessageProtocol


class NetworkServer:
    def __init__(self, host, port, handler):
        self.host = host
        self.port = port
        self.handler = handler
        self.server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.server_socket.bind((self.host, self.port))
        self.server_socket.listen()
        self.running = False
        self.thread = None

    def start(self):
        self.running = True
        self.thread = threading.Thread(target=self.run)
        self.thread.start()

    def stop(self):
        self.running = False
        if self.thread:
            self.thread.join()

    def run(self):
        while self.running:
            client_socket, address = self.server_socket.accept()
            threading.Thread(target=self.handle_client, args=(client_socket,)).start()

    def handle_client(self, client_socket):
        while self.running:
            data = client_socket.recv(1024)
            if data:
                self.handler(client_socket, data)


class P2PServer:
    def __init__(self, node, host: str, port: int):
        self.node = node
        self.host = host
        self.port = port
        self.network_server = NetworkServer(host, port, self.handle_message)
        self.running = False
        self.message_protocol = MessageProtocol()
        self.chat_queue = queue.Queue()  # Queue for storing chat messages

    def start(self):
        self.running = True
        self.network_server.start()

    def stop(self):
        self.running = False
        self.network_server.stop()

    def handle_message(self, client_socket, data):
        message = self.message_protocol.decode_message(data)
        message_type = message["type"]
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

    def read_next_message(self):
        return self.chat_queue.get()
