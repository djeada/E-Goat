import socket
from typing import Optional


class NetworkClient:
    """
    NetworkClient is the low-level network client that handles networking.
    It opens a connection to a server, sends a message, and closes the connection.

    Args:
        None
    """

    def __init__(self):
        self.client_socket: Optional[socket.socket] = None

    def connect(self, host: str, port: int) -> None:
        """
        Connects the client to a given host and port.

        Args:
            host (str): The hostname or IP address of the server to connect to.
            port (int): The port number of the server to connect to.
        """
        self.client_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.client_socket.connect((host, port))

    def send(self, message: bytes) -> None:
        """
        Sends a message to the server.

        Args:
            message (bytes): The message to send.
        """
        self.client_socket.sendall(message)

    def close(self) -> None:
        """
        Closes the client socket.

        Args:
            None
        """
        if self.client_socket is not None:
            self.client_socket.close()
            self.client_socket = None
