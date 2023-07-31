import json
from typing import Dict, Any


class MessageProtocol:
    """
    This class represents the protocol for sending messages in the P2P network.
    It provides methods to encode and decode messages using JSON.
    """

    def encode_message(self, message: Dict[str, Any]) -> bytes:
        """
        Encodes a message into JSON format and then encodes it to bytes.

        Args:
            message: A dictionary representing the message.

        Returns:
            The encoded message as bytes.
        """
        return json.dumps(message).encode()

    def decode_message(self, data: bytes) -> Dict[str, Any]:
        """
        Decodes a message from bytes to JSON format and then converts it to a dictionary.

        Args:
            data: The message data in bytes format.

        Returns:
            The decoded message as a dictionary.
        """
        return json.loads(data.decode())
