"""
Command line interface for interacting with a P2P node.
"""
import cmd
import threading
import time
from typing import Optional

from src.lib.p2p.p2p_node import P2PNode


class NodeCLI(cmd.Cmd):
    """
    Command line interface for interacting with a P2P node.

    Attributes:
        node: The P2PNode that this interface interacts with.
        active: A boolean indicating whether or not the node is active.
    """

    intro = "Welcome to the P2P node. Type help or ? to list commands.\n"
    prompt = "(node-cli) "

    def __init__(self, host: str, port: int):
        """
        Initialize a new NodeCLI instance.

        Args:
            host: The host address for the node.
            port: The port number for the node.
        """
        super().__init__()
        self.node: P2PNode = P2PNode(host, port)
        self.active: bool = False

    def preloop(self):
        """Prepare the command loop by starting the node."""
        self.do_start(None)

    def do_start(self, arg: str) -> None:
        """
        Start the node.

        Args:
            arg: Command arguments (unused).
        """
        print("Starting node...")
        self.node.start()
        self.active = True
        threading.Thread(target=self.receive_messages, daemon=True).start()

    def do_stop(self, _: str) -> None:
        """
        Stop the node.

        Args:
            _: Command arguments (unused).
        """
        print("Stopping node...")
        self.node.stop()
        self.active = False

    def do_connect(self, arg: str) -> None:
        """
        Connect to a peer.

        Args:
            arg: Command arguments in format "<host> <port>".
        """
        args = arg.split()
        if len(args) != 2:
            print("Invalid number of arguments.")
            return

        host, port_str = args
        try:
            port = int(port_str)
        except ValueError:
            print("Invalid port number.")
            return

        if self.node.connect_to_peer(host, port):
            print(f"Successfully connected to {host}:{port}")
        else:
            print(f"Could not connect to {host}:{port}")

    def do_send(self, arg: str) -> None:
        """
        Send a message.

        Args:
            arg: Command arguments, i.e., the message to be sent.
        """
        if not arg:
            print("Message cannot be empty.")
            return

        self.node.broadcast_message(arg)
        print("Message sent!")

    def receive_messages(self) -> None:
        """
        Continuously check for and print any received messages while the node is active.
        """
        while self.active:
            message: Optional[str] = self.node.read_next_message()
            if message:
                print(f"\nReceived message: {message}")
                print(self.prompt, end="")  # reprint prompt
            time.sleep(0.1)
