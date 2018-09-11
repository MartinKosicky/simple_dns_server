# simple_dns_server
Simple dns server for A records without recursion. Source is by an executable

Run the dnsserver executable with the following:  dnsserver command [arguments...]
command is an executable that will be periodically called with the specified arguments to refresh the ip/hostname cache.

dnsserver.exe cmd /C "echo 33.11.22.33;www.something.com"
  
