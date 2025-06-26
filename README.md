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

Manually cleanup expired bans (soft delete):
`go run main.go cleanup`

Remove old records from DB (thirty days):
`go run main.go purge 30`

For more days to keep, you can change 30 to like 60:
`go run main.go purge 60`

Manually ban/Add a IP:
`go run main.go add 192.168.1.100 24h "Brute force attack" '{"severity":"high","source":"fail2ban"}'`

Perma ban IP:
`go run main.go add 192.168.1.102 permanent "Serious threat" '{"threat_level":"critical"}'`

Manually unban a IP:
`go run main.go del 192.168.1.100`

IP is ofcourse a dummy and go run main.go can be replaced with the binary like: `./bitninja-manager` for the local directory

To use it with crowdsec enter the binary location in the proper location of the custom bouncer.

It is also possible to cleanup bans on a schedule, to do so, add the following to tools like crontab.
```# Every day at 2:00 AM - cleanup expired bans (soft ban)
0 2 * * * /path/to/your/script cleanup
```
---
```# Every week on zondag at 3:00 AM - purge old records (30 dagen)
0 3 * * 0 /path/to/your/script purge 30
```
---
Same as before, you can change 30 to a custom value like 60
`0 3 * * 0 /path/to/your/script purge 60`
