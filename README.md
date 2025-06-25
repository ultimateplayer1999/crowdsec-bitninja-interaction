This adds all Crowdsec blocked IPs to the local Bitninja blocklist. Requirements, Crowdsec and Bitninja on the same machine

This needs some dependencies, these are mongoDB and dotenv. This can be installed with:
---
MongoDB:
`go get go.mongodb.org/mongo-driver/mongo`
---
Dotenv:
`go get github.com/joho/godotenv`

To get it working, copy example.env to .env and change the values when needed. Otherwise it uses the default.

To run it, you can use `go run main list` to receive all active bans. To build a local executable, you can use 
`go build -o bitninja-manager main.go`

to build it as bitninja-manager

The available commands are:

Receive all currently banned IPs (logged):
`go run main.go list`

Get stats:
`go run main.go stats`

Manually cleanup expired bans:
`go run main.go cleanup`

Manually ban/Add a IP:
`go run main.go add 192.168.1.100 24h "Brute force attack" '{"severity":"high","source":"fail2ban"}'`

Perma ban IP:
`go run main.go add 192.168.1.102 permanent "Serious threat" '{"threat_level":"critical"}'`

Manually unban a IP:
`go run main.go del 192.168.1.100`

IP is ofcourse a dummy and go run main.go can be replaced with the binary like: `./bitninja-manager` for the local directory
