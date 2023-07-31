import pytest
import socket
import threading

from src.lib.p2p.client.network_client import NetworkClient


class EchoServer:
    def __init__(self, host, port):
        self.host = host
        self.port = port
        self.server_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.server_socket.bind((self.host, self.port))
        self.server_socket.listen(1)
        self.running = False
        self.last_received_message = None

    def start(self):
        self.running = True
        self.thread = threading.Thread(target=self.run)
        self.thread.start()

    def stop(self):
        self.running = False
        socket.create_connection((self.host, self.port))
        self.thread.join()

    def run(self):
        while self.running:
            try:
                client_socket, address = self.server_socket.accept()
                data = client_socket.recv(1024)
                if data:
                    self.last_received_message = data
                    client_socket.sendall(data)
                client_socket.close()
            except:
                if self.running:
                    raise
                else:
                    return

    def get_last_received_message(self):
        return self.last_received_message


@pytest.fixture(scope="module")
def server():
    server = EchoServer("localhost", 12345)
    server.start()
    yield server
    server.stop()


def test_network_client(server):
    client = NetworkClient()
    client.connect("localhost", 12345)
    message = b"Hello, Server!"
    client.send(message)
    client.close()

    assert server.get_last_received_message() == message
