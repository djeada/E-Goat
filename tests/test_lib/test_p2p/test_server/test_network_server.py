import pytest
import socket
import time

from src.lib.p2p.server.network_server import NetworkServer


def echo_handler(data):
    return data


@pytest.fixture(scope="module")
def server():
    server = NetworkServer("localhost", 12345, echo_handler)
    server.start()
    yield server
    server.stop()


def test_server_starts_and_stops(server):
    assert server.running


def test_server_accepts_connections(server):
    client_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        client_socket.connect(("localhost", 12345))
    except Exception as e:
        pytest.fail(f"Should not have raised any exception, but got {e}")
    finally:
        client_socket.close()


def test_server_handle_connections(server):
    received_data = None

    def test_handler(data):
        nonlocal received_data
        received_data = data

    server.handler = test_handler

    client_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        client_socket.connect(("localhost", 12345))
        client_socket.sendall(b"Hello, Server!")
        time.sleep(0.5)  # allow data to be handled by server
        assert received_data == b"Hello, Server!"
    except Exception as e:
        pytest.fail(f"Should not have raised any exception, but got {e}")
    finally:
        client_socket.close()
