import sys
import threading
from p2p_peer import P2PNode  

class P2PNodeRunner:
    def __init__(self, host: str, port: int):
        self.node = P2PNode(host, port)

    def start(self):
        threading.Thread(target=self.node.start_server).start()
        self.handle_user_input()

    def handle_user_input(self):
        while True:
            command = input('Enter command or message (type 0 to quit): ')
            if command == '0':
                break
            elif command == 'connect':
                host = input('Enter host to connect to: ')
                port = int(input('Enter port to connect to: '))
                if self.node.connect_to_peer(host, port):
                    print("Connection established successfully!")
                else:
                    print("Failed to establish connection.")
            else:
                self.node.broadcast_message(command.encode())


def main():
    port = int(input("Enter your port: "))
    node_runner = P2PNodeRunner('localhost', port)
    node_runner.start()

if __name__ == "__main__":
    main()
