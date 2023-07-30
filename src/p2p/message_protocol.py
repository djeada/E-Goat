import json


class MessageProtocol:
    def encode_message(self, message):
        return json.dumps(message).encode()

    def decode_message(self, data):
        return json.loads(data.decode())
