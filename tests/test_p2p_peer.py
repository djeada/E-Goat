import pytest
from src.p2p_peer import P2PNode


def test_p2pnode_instantiation():

    with P2PNode("localhost", 5000) as node:
        assert isinstance(node, P2PNode)


def test_start_method():
    with P2PNode("localhost", 5000) as node:
        # We may not have a direct way to assert thread start, so just assert that 'running' is True
        assert node.running == True


def test_stop_method():
    with P2PNode("localhost", 5000) as node:
        pass

    assert node.running == False


def test_send_message():
    with P2PNode("localhost", 5000) as node:
        message_type = "test"
        data = {"test": "data"}
        expected_json = '{"type": "test", "data": {"test": "data"}}'
        # Mock a peer_socket
        class MockSocket:
            def sendall(self, message):
                self.sent_message = message

        peer_socket = MockSocket()
        node.send_message(peer_socket, message_type, data)
        assert peer_socket.sent_message.decode() == expected_json


def test_connect_to_peer(mocker):
    with P2PNode("localhost", 5000) as node:
        assert node.connect_to_peer("127.0.0.1", 5001)  # should return True

        # Check the peer has been added
        assert len(node.peers) == 1
        assert node.peers[0].getpeername() == ("127.0.0.1", 5001)


def test_handle_peer(mocker):
    # Mock a client_socket
    class MockSocket:
        def __init__(self, recv_message):
            self.recv_message = recv_message
            self.message_sent = False

        def recv(self, _):
            if not self.message_sent:
                self.message_sent = True
                return self.recv_message.encode()
            else:
                return "".encode()

        def getpeername(self):
            return "localhost", 5001

        def shutdown(self, _):
            pass

        def close(self):
            pass

    # Mock a client_socket that sends a chat message
    client_socket = MockSocket('{"type": "chat", "data": "Hello!"}')

    # Mock the broadcast_message method so it doesn't actually try to send anything
    mocker.patch.object(P2PNode, "broadcast_message")

    with P2PNode("localhost", 5000) as node:
        # Add the mock client_socket to the peers list
        with node.peers_lock:
            node.peers.append(client_socket)

        node.handle_peer(client_socket)

        # Since we can't directly check the messages received, check that broadcast_message was called
        P2PNode.broadcast_message.assert_called_once()


def test_broadcast_message(mocker):
    # Mock a client_socket
    class MockSocket:
        def sendall(self, message):
            self.sent_message = message

        def getpeername(self):
            return "localhost", 5001

        def shutdown(self, _):
            pass

        def close(self):
            pass

    peer_socket1 = MockSocket()
    peer_socket2 = MockSocket()
    sender_socket = MockSocket()

    with P2PNode("localhost", 5000) as node:
        node.peers = [peer_socket1, peer_socket2, sender_socket]

        message = '{"type": "chat", "data": "Hello!"}'
        node.broadcast_message(message, sender_socket)

        # Check that the message was sent to the other peers, but not the sender
        assert peer_socket1.sent_message.decode() == message
        assert peer_socket2.sent_message.decode() == message
        assert hasattr(sender_socket, "sent_message") == False
