import argparse

from src.apps.cli.node_cli import NodeCLI


def run_cli(host: str, port: int) -> None:
    """
    Starts the command line interface for the P2P Node.

    Args:
        host: The host address of the node.
        port: The port number of the node.
    """
    node_cli = NodeCLI(host, port)
    node_cli.cmdloop()


def run_gui(host: str, port: int) -> None:
    """
    Starts the graphical user interface for the P2P Node.

    Args:
        host: The host address of the node.
        port: The port number of the node.
    """
    raise NotImplementedError("GUI option is not implemented yet")
    # node_gui = NodeGUI(host, port)
    # node_gui.mainloop()  # Assuming your GUI has a mainloop method


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="CLI for P2P Node")
    parser.add_argument("host", help="Host of the P2P node")
    parser.add_argument("port", type=int, help="Port for the P2P node")
    parser.add_argument(
        "--mode",
        choices=["cli", "gui"],
        default="cli",
        help="Choose the mode: CLI (default) or GUI",
    )
    args = parser.parse_args()

    if args.mode == "gui":
        run_gui(args.host, args.port)
    else:
        run_cli(args.host, args.port)
