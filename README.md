# rdpblast
This is a program written in GO that works in any Debian systems, that uses a list of IP addresses or a single IP address that have an open RDP 3389 port to bruteforce credentials. Credentials can be provided in a list in a username:password format. The goal of this program is to test a mass amount of IP addresses withing a CIDR that dont enforce a Certificate for RDP connections.

# Installing
Download GO from the official website at https://go.dev/dl/ and install the GO language. 

Once packages are downloaded, start building the program:

bash:

	chmod +x build.sh
	./build.sh
    
OR to install all dependencies: 
    
    sudo apt-get install -y golang-go rdesktop xvfb imagemagick 

then build it with go:
    
    go build -ldflags="-s -w" -o rdpblast
