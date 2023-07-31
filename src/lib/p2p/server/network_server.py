import socket
import threading
from typing import Callable


class NetworkServer:
    """
    NetworkServer is the low-level server that handles networking.
    It accepts connections and then spawns a new thread to handle each connection,
    calling a provided handler function with the client socket and the message.

    Args:
        host (str): The hostname or IP address on which the server is listening.
        port (int): The port number on which the server is listening.
        handler (Callable): The handler function to be called when a client connection is established.
    """

    def __init__(self, host: str, port: int, handler: Callable):
        self.host = host
        self.port = port
        self.handler = handler
        self.server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.server_socket.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR,
                                      1)
        self.server_socket.bind((self.host, self.port))
        self.server_socket.listen()
        self.running = False
        self.thread = None

    def start(self) -> None:
        """Starts the server and begin listening for connections."""
        self.running = True
        self.thread = threading.Thread(target=self.run)
        self.thread.start()

    def stop(self) -> None:
        """Stops the server and clean up the resources."""
        self.running = False
        # Create a dummy connection to unblock the accept()
        socket.create_connection((self.host, self.port))
        if self.thread:
            self.thread.join(timeout=1)

    def run(self) -> None:
        """Runs the server, continuously accepting new client connections."""
        while self.running:
            try:
                client_socket, address = self.server_socket.accept()
                threading.Thread(target=self.handle_client,
                                 args=(client_socket,)).start()
            except Exception as e:
                pass

    def handle_client(self, client_socket: socket.socket) -> None:
        """Handles a client connection.

        Args:
            client_socket (socket.socket): The client socket.
        """
        while self.running:
            data = client_socket.recv(1024)
            if data:
                self.handler(data)
