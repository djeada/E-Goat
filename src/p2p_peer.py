import socket
import threading

class P2PNode:
    def __init__(self, host: str, port: int):
        self.host = host
        self.port = port
        self.server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.peers = []

    def start(self):
        threading.Thread(target=self.start_server).start()

    def start_server(self):
        self.server_socket.bind((self.host, self.port))
        self.server_socket.listen()
        print(f"Server started on {self.host}:{self.port}")
        while True:
            client_socket, _ = self.server_socket.accept()
            self.peers.append(client_socket)
            threading.Thread(target=self.handle_peer, args=(client_socket,)).start()

    def connect_to_peer(self, host: str, port: int):
        client_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        try:
            client_socket.connect((host, port))
            self.peers.append(client_socket)
            threading.Thread(target=self.handle_peer, args=(client_socket,)).start()
            return True
        except ConnectionRefusedError:
            return False

    def handle_peer(self, client_socket: socket.socket):
        print(f"New connection established with {client_socket.getpeername()}")
        while True:
            try:
                message = client_socket.recv(1024)
                if not message:
                    break
                print(f"Received message from {client_socket.getpeername()}: {message.decode()}")
                self.broadcast_message(message, sender_socket=client_socket)
            except ConnectionResetError:
                break
        self.peers.remove(client_socket)
        client_socket.close()

    def broadcast_message(self, message: bytes, sender_socket: socket.socket = None):
        for peer in self.peers:
            if peer != sender_socket:
                peer.sendall(message)
