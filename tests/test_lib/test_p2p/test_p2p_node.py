import pytest
from unittest.mock import patch

from src.lib.p2p.p2p_node import Peer, P2PNode


@pytest.fixture(scope="module")
def mock_peer():
    return Peer("localhost", 5001)


@pytest.fixture(scope="module")
def p2p_node():
    return P2PNode("localhost", 5000)


def test_add_peer(p2p_node, mock_peer):
    with patch.object(p2p_node, "broadcast_message") as mock_broadcast:
        p2p_node.add_peer(mock_peer)

    assert mock_peer in p2p_node.peers
    mock_broadcast.assert_called_with(
        p2p_node.get_serializable_peers(), message_type="update_peers"
    )


def test_remove_peer(p2p_node, mock_peer):
    p2p_node.peers.add(mock_peer)
    with patch.object(p2p_node, "broadcast_message") as mock_broadcast:
        p2p_node.remove_peer("localhost", 5001)

    assert mock_peer not in p2p_node.peers
    mock_broadcast.assert_called_with(
        p2p_node.get_serializable_peers(), message_type="update_peers"
    )


def test_connect_to_peer(p2p_node, mock_peer):
    with patch.object(p2p_node.client, "send_message") as mock_send:
        p2p_node.connect_to_peer("localhost", 5001)

    assert mock_peer in p2p_node.peers
    mock_send.assert_called()


def test_get_serializable_peers(p2p_node, mock_peer):
    p2p_node.peers.add(mock_peer)
    serialized_peers = p2p_node.get_serializable_peers()
    assert set(serialized_peers) == set([("localhost", 5000), ("localhost", 5001)])


def test_update_peers(p2p_node, mock_peer):
    with patch.object(p2p_node.client, "send_message") as mock_send:
        p2p_node.update_peers([("localhost", 5002)])

    assert ("localhost", 5002) in p2p_node.get_serializable_peers()


def test_broadcast_message(p2p_node, mock_peer):
    p2p_node.peers.clear()
    p2p_node.peers.add(mock_peer)
    with patch.object(p2p_node.client, "send_message") as mock_send:
        p2p_node.broadcast_message("Hello!")

    mock_send.assert_called_with(mock_peer.host, mock_peer.port, "chat", "Hello!")
