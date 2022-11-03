# E-Goat
Real-time chat application built on peer-to-peer network architecture and the TCP protocol.

* http://web.mit.edu/6.005/www/fa15/classes/21-sockets-networking/
* https://github.com/shashwatdixit124/IPConnect
* https://iximiuz.com/en/posts/writing-web-server-in-python-sockets/

# Can you use it to communicate with devices outside your LAN?

you may be running into an ISP problem because you have a residential network, and they all have a clause in the terms of use that forbid running a service to the Internet, and some ISPs actively block that. 

 In any case, to use IPv4, you must have an IPv4 WAN address on your router, otherwise it seems your ISP is using NAT, so you cannot forward a port and use IPv4. For IPv6, you use the Global IPv6 address assigned to your server, not the WAN address. 


# Bibliography

* https://w3.cs.jmu.edu/kirkpams/OpenCSF/Books/csf/html/

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate.

## License
[MIT](https://choosealicense.com/licenses/mit/)
