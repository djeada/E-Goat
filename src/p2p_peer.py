import socket
import threading
class P2PNode:
    def __init__(self, host: str, port: int):
        self.host = host
        self.port = port
        self.server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.peers = []
        self.running = True

    def start(self):
        threading.Thread(target=self.start_server).start()

    def stop(self):
        self.running = False
        self.server_socket.close()
        for peer in self.peers:
            peer.close()
        print("Server has been shut down")

    def start_server(self):
        try:
            self.server_socket.bind((self.host, self.port))
            self.server_socket.listen()
            print(f"Server started on {self.host}:{self.port}")
            while self.running:
                client_socket, _ = self.server_socket.accept()
                self.peers.append(client_socket)
                threading.Thread(target=self.handle_peer, args=(client_socket,)).start()
        except Exception as e:
            print(f"Server encountered an error: {e}")
            self.stop()

    def connect_to_peer(self, host: str, port: int):
        client_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        try:
            client_socket.connect((host, port))
            self.peers.append(client_socket)
            threading.Thread(target=self.handle_peer, args=(client_socket,)).start()
            return True
        except ConnectionRefusedError:
            print("Failed to establish connection, connection refused.")
            return False
        except Exception as e:
            print(f"Failed to establish connection due to error: {e}")
            return False

    def handle_peer(self, client_socket: socket.socket):
        print(f"New connection established with {client_socket.getpeername()}")
        while self.running:
            try:
                message = client_socket.recv(1024)
                if not message:
                    break
                print(f"Received message from {client_socket.getpeername()}: {message.decode()}")
                self.broadcast_message(message, sender_socket=client_socket)
            except ConnectionResetError:
                print("Connection reset by peer.")
                break
            except Exception as e:
                print(f"Error occurred while handling peer: {e}")
                break
        self.peers.remove(client_socket)
        client_socket.close()

    def broadcast_message(self, message: bytes, sender_socket: socket.socket = None):
        for peer in self.peers:
            if peer != sender_socket:
                try:
                    peer.sendall(message)
                except Exception as e:
                    print(f"Error occurred while broadcasting message: {e}")
