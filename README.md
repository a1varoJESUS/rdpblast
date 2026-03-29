# rdpblast
This is a program written in GO that works in any Debian systems, that uses a list of IP addresses or a single IP address that have an open RDP 3389 port to bruteforce credentials. Credentials can be provided in a list in a username:password format. The goal of this program is to test a mass amount of IP addresses withing a CIDR that dont enforce a Certificate for RDP connections.

# Installing
Download GO from the official website at https://go.dev/dl/ and install the GO language. 

Once packages are downloaded save them into a new directory, then navigate to that folder and start building the program:

bash:

	chmod +x build.sh
	./build.sh
    
OR to install all dependencies: 
    
    sudo apt-get install -y golang-go rdesktop xvfb imagemagick 

then build it with go:
    
    go build -ldflags="-s -w" -o rdpblast

# Usage
Run using bash:

	./rdpblast 

	rdpblast — RDP credential tester (rdesktop)

	Usage: rdpblast -t <host> -f <wordlist> [options]

	  -t  string   Target IP / hostname          (required)
	  -f  string   Credentials file user:pass    (required)
	  -p  int      RDP port              (default 3389)
	  -d  string   Windows domain        (optional)
	  -n  int      Threads               (default 1)
	  -o  string   Screenshot dir        (default /home/kelevran/screenshot)

