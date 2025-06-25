This adds all Crowdsec blocked IPs to the local Bitninja blocklist. Requirements, Crowdsec and Bitninja on the same machine

This needs some dependencies, these are mongoDB and dotenv. This can be installed with:
MongoDB:
`go get go.mongodb.org/mongo-driver/mongo`
Dotenv:
`go get github.com/joho/godotenv`

To get it working, copy example.env to .env and change the values when needed. Otherwise it uses the default.
