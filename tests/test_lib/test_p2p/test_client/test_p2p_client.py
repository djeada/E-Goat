from unittest.mock import patch, Mock
from src.lib.p2p.client.p2p_client import P2PClient
from src.lib.p2p.client.network_client import NetworkClient


def test_send_message():
    mock_node = Mock()
    p2p_client = P2PClient(mock_node)

    with patch("threading.Thread.start"), patch.object(
        p2p_client.message_protocol, "encode_message"
    ) as mock_encode:
        mock_encode.return_value = b"encoded_message"
        p2p_client.send_message("localhost", 5000, "message_type", "data")

        # check if message_protocol.encode_message is called correctly
        mock_encode.assert_called_with({"type": "message_type", "data": "data"})


def test_connect_and_send_success():
    mock_node = Mock()
    p2p_client = P2PClient(mock_node)

    with patch.object(NetworkClient, "connect") as mock_connect, patch.object(
        NetworkClient, "send"
    ) as mock_send, patch.object(NetworkClient, "close") as mock_close:
        mock_connect.return_value = None
        mock_send.return_value = None
        mock_close.return_value = None

        p2p_client.connect_and_send("localhost", 5000, b"encoded_message")

    # check if network client's methods were called correctly
    mock_connect.assert_called_with("localhost", 5000)
    mock_send.assert_called_with(b"encoded_message")
    mock_close.assert_called()


def test_connect_and_send_failure():
    mock_node = Mock()
    p2p_client = P2PClient(mock_node)

    with patch.object(NetworkClient, "connect") as mock_connect, patch.object(
        NetworkClient, "close"
    ) as mock_close, patch.object(p2p_client.node, "remove_peer") as mock_remove_peer:
        mock_connect.side_effect = ConnectionRefusedError
        mock_close.return_value = None

        p2p_client.connect_and_send("localhost", 5000, b"encoded_message")

    # check if node.remove_peer was called when a connection failure occurs
    mock_remove_peer.assert_called_with("localhost", 5000)
