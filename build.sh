#!/bin/bash
# build.sh — Build rdpblast (rdesktop edition) on Debian
set -e
 
echo "[*] Installing dependencies..."
sudo apt-get update -qq
sudo apt-get install -y golang-go rdesktop xvfb imagemagick
 
echo "[*] Building rdpblast..."
cd "$(dirname "$0")"
go build -ldflags="-s -w" -o rdpblast .
 
echo ""
echo "[+] Build complete: ./rdpblast"
echo ""
echo "Quick start:"
echo "  ./rdpblast -t 192.168.1.50 -f creds.txt"
echo "  ./rdpblast -t 192.168.1.50 -f creds.txt -n 5 -d CORP"
