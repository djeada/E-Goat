from unittest import mock

import pytest

from src.lib.p2p.server.network_server import NetworkServer
from src.lib.p2p.server.p2p_server import P2PServer


@pytest.fixture(scope="module")
def mock_node():
    """Creates a mock Node."""
    return mock.MagicMock()


@pytest.fixture(scope="module")
def p2p_server(mock_node):
    """Creates a P2PServer instance with a mock node."""
    server = P2PServer(mock_node, "localhost", 12345)
    server.network_server = mock.MagicMock(spec=NetworkServer)
    server.message_protocol.decode_message = lambda x: {
        "type": "chat",
        "data": "Hello, peer!",
    }
    return server


def test_start(p2p_server):
    p2p_server.start()
    assert p2p_server.running
    p2p_server.network_server.start.assert_called_once()


def test_stop(p2p_server):
    p2p_server.start()
    p2p_server.stop()
    assert not p2p_server.running
    p2p_server.network_server.stop.assert_called_once()


def test_handle_message_chat(p2p_server):
    message = b"Hello, peer!"
    p2p_server.handle_message(message)
    assert p2p_server.chat_queue.get() == "Hello, peer!"


def test_handle_message_request_peers(p2p_server, mock_node):
    message = {"type": "request_peers", "data": None}
    p2p_server.message_protocol.decode_message = lambda x: message
    p2p_server.handle_message(b"Request peers.")
    mock_node.broadcast_message.assert_called_once()


def test_handle_message_update_peers(p2p_server, mock_node):
    message = {"type": "update_peers", "data": ["Peer1", "Peer2"]}
    p2p_server.message_protocol.decode_message = lambda x: message
    p2p_server.handle_message(b"Update peers.")
    mock_node.update_peers.assert_called_once_with(["Peer1", "Peer2"])


def test_read_next_message(p2p_server):
    p2p_server.chat_queue.put("Hello, peer!")
    assert p2p_server.read_next_message() == "Hello, peer!"
