import argparse
import threading
import time
import queue
from p2p.node import P2PNode


class NodeCLI:
    def __init__(self, host: str, port: int):
        self.node = P2PNode(host, port)
        self.active = False

    def start(self):
        print("Starting node...")
        self.node.start()
        self.active = True
        threading.Thread(target=self.receive_messages, daemon=True).start()
        self.run_cli()

    def stop(self):
        print("Stopping node...")
        self.node.stop()
        self.active = False

    def run_cli(self):
        try:
            host = "localhost"
            port = int(input("Enter port to connect: ").strip())

            if self.node.connect_to_peer(host, port):
                print(f"Successfully connected to {host}:{port}")
            else:
                print(f"Could not connect to {host}:{port}")
        except:
            pass
        while self.active:
            message = input("Enter message to send: ").strip()
            self.node.broadcast_message(message)

    def receive_messages(self):
        while self.active:
            message = self.node.read_next_message()
            if message:
                print(f"Received message: {message}")
            time.sleep(0.1)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="CLI for P2P Node")
    parser.add_argument("host", help="Host of the P2P node")
    parser.add_argument("port", type=int, help="Port for the P2P node")
    args = parser.parse_args()

    node_cli = NodeCLI(args.host, args.port)
    node_cli.start()
