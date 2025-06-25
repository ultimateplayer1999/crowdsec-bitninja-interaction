package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/joho/godotenv"
)

type BanRecord struct {
	ID        primitive.ObjectID     `bson:"_id,omitempty" json:"id,omitempty"`
	IP        string                 `bson:"ip" json:"ip"`
	Reason    string                 `bson:"reason" json:"reason"`
	BannedAt  time.Time              `bson:"banned_at" json:"banned_at"`
	ExpiresAt *time.Time             `bson:"expires_at,omitempty" json:"expires_at,omitempty"`
	Duration  string                 `bson:"duration" json:"duration"`
	JsonData  map[string]interface{} `bson:"json_data" json:"json_data"`
	Status    string                 `bson:"status" json:"status"`
	Hostname  string                 `bson:"hostname" json:"hostname"`
}

type MongoConfig struct {
	URI        string
	Database   string
	Collection string
}

func getMongoConfig() MongoConfig {
	// Load environment variables from .env file
	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	return MongoConfig{
		URI:        getEnvOrDefault("MONGO_URI", "mongodb://localhost:27017"),
		Database:   getEnvOrDefault("MONGO_DATABASE", "bitninja"),
		Collection: getEnvOrDefault("MONGO_COLLECTION", "ban_records"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func connectMongoDB(config MongoConfig) (*mongo.Client, *mongo.Collection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.URI))
	if err != nil {
		return nil, nil, err
	}

	// Test connection
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, nil, err
	}

	collection := client.Database(config.Database).Collection(config.Collection)

	// Create indexes for better performance
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "ip", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "expires_at", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "expires_at", Value: 1},
			},
		},
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		log.Printf("Warning: Could not create indexes: %v", err)
	}

	return client, collection, nil
}

func parseDuration(duration string) (time.Duration, error) {
	if duration == "permanent" || duration == "0" {
		return 0, nil // 0 means permanent
	}

	if strings.HasSuffix(duration, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(duration, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(duration)
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func addBanRecord(collection *mongo.Collection, ip, reason, duration string, jsonData map[string]interface{}) error {
	d, err := parseDuration(duration)
	if err != nil {
		return err
	}

	now := time.Now()
	var expiresAt *time.Time

	if d > 0 { // Not permanent
		expiry := now.Add(d)
		expiresAt = &expiry
	}

	record := BanRecord{
		IP:        ip,
		Reason:    reason,
		BannedAt:  now,
		ExpiresAt: expiresAt,
		Duration:  duration,
		JsonData:  jsonData,
		Status:    "active",
		Hostname:  getHostname(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = collection.InsertOne(ctx, record)
	return err
}

func removeBanRecord(collection *mongo.Collection, ip string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"ip":     ip,
		"status": "active",
	}

	update := bson.M{
		"$set": bson.M{
			"status": "removed",
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	return err
}

func cleanupExpiredBans(collection *mongo.Collection) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Find expired bans
	filter := bson.M{
		"expires_at": bson.M{"$lte": time.Now()},
		"status":     "active",
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var expiredBans []BanRecord
	if err = cursor.All(ctx, &expiredBans); err != nil {
		return err
	}

	// Remove expired IPs from BitNinja and mark as expired in MongoDB
	for _, ban := range expiredBans {
		fmt.Printf("Removing expired ban for IP: %s (banned at: %s)\n", 
			ban.IP, ban.BannedAt.Format("2006-01-02 15:04:05"))

		// Remove from BitNinja
		cmd := exec.Command("bitninjacli", "--blacklist", fmt.Sprintf("--del=%s", ban.IP))
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error removing IP %s from BitNinja: %v\n", ban.IP, err)
			continue
		}

		// Mark as expired in MongoDB
		updateFilter := bson.M{"_id": ban.ID}
		update := bson.M{
			"$set": bson.M{
				"status": "expired",
			},
		}

		_, err = collection.UpdateOne(ctx, updateFilter, update)
		if err != nil {
			fmt.Printf("Error updating MongoDB for IP %s: %v\n", ban.IP, err)
		}
	}

	fmt.Printf("Processed %d expired bans\n", len(expiredBans))
	return nil
}

func handleAdd(collection *mongo.Collection, ip string, duration string, reason string, jsonObject map[string]interface{}) {
	// Add to BitNinja blacklist
	enhancedReason := fmt.Sprintf("%s (Duration: %s, Host: %s)", reason, duration, getHostname())
	cmd := exec.Command("bitninjacli", "--blacklist", fmt.Sprintf("--add=%s", ip), fmt.Sprintf("--comment=%s", enhancedReason))
	out, err := cmd.Output()
	if err != nil {
		fmt.Println("Error adding IP to BitNinja:", err)
		return
	}
	fmt.Println(string(out))

	// Add to MongoDB
	err = addBanRecord(collection, ip, reason, duration, jsonObject)
	if err != nil {
		fmt.Printf("Error adding to MongoDB: %v\n", err)
		return
	}

	if duration != "permanent" && duration != "0" {
		d, _ := parseDuration(duration)
		fmt.Printf("IP %s added to blacklist, will expire at: %s\n", ip, time.Now().Add(d).Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("IP %s added to blacklist permanently\n", ip)
	}
}

func handleDel(collection *mongo.Collection, ip string, duration string, reason string, jsonObject map[string]interface{}) {
	// Remove from BitNinja
	cmd := exec.Command("bitninjacli", "--blacklist", fmt.Sprintf("--del=%s", ip))
	out, err := cmd.Output()
	if err != nil {
		fmt.Println("Error deleting IP from BitNinja:", err)
		return
	}
	fmt.Println(string(out))

	// Update MongoDB
	err = removeBanRecord(collection, ip)
	if err != nil {
		fmt.Printf("Error updating MongoDB: %v\n", err)
	} else {
		fmt.Printf("IP %s removed from tracking database\n", ip)
	}
}

func handleCleanup(collection *mongo.Collection) {
	fmt.Println("Cleaning up expired bans...")
	err := cleanupExpiredBans(collection)
	if err != nil {
		fmt.Printf("Error during cleanup: %v\n", err)
	} else {
		fmt.Println("Cleanup completed")
	}
}

func handleList(collection *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find active bans, sorted by banned_at descending
	filter := bson.M{"status": "active"}
	opts := options.Find().SetSort(bson.D{{Key: "banned_at", Value: -1}})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		fmt.Printf("Error querying MongoDB: %v\n", err)
		return
	}
	defer cursor.Close(ctx)

	var bans []BanRecord
	if err = cursor.All(ctx, &bans); err != nil {
		fmt.Printf("Error decoding results: %v\n", err)
		return
	}

	fmt.Println("\nActive Bans:")
	fmt.Printf("%-15s %-20s %-19s %-19s %-10s %-12s\n", "IP", "Reason", "Banned At", "Expires At", "Duration", "Hostname")
	fmt.Println(strings.Repeat("-", 100))

	for _, ban := range bans {
		expiryStr := "Permanent"
		if ban.ExpiresAt != nil {
			expiryStr = ban.ExpiresAt.Format("2006-01-02 15:04")
		}

		reasonTrunc := ban.Reason
		if len(reasonTrunc) > 20 {
			reasonTrunc = reasonTrunc[:17] + "..."
		}

		fmt.Printf("%-15s %-20s %-19s %-19s %-10s %-12s\n",
			ban.IP,
			reasonTrunc,
			ban.BannedAt.Format("2006-01-02 15:04"),
			expiryStr,
			ban.Duration,
			ban.Hostname)
	}

	fmt.Printf("\nTotal active bans: %d\n", len(bans))
}

func handleStats(collection *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Count by status
	pipeline := []bson.M{
		{"$group": bson.M{
			"_id":   "$status",
			"count": bson.M{"$sum": 1},
		}},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		fmt.Printf("Error getting stats: %v\n", err)
		return
	}
	defer cursor.Close(ctx)

	fmt.Println("\nBan Statistics:")
	for cursor.Next(ctx) {
		var result bson.M
		if err := cursor.Decode(&result); err != nil {
			continue
		}
		fmt.Printf("  %s: %v\n", result["_id"], result["count"])
	}

	// Count expiring soon (next 24 hours)
	tomorrow := time.Now().Add(24 * time.Hour)
	expiringSoonFilter := bson.M{
		"status": "active",
		"expires_at": bson.M{
			"$gte": time.Now(),
			"$lte": tomorrow,
		},
	}

	count, err := collection.CountDocuments(ctx, expiringSoonFilter)
	if err == nil {
		fmt.Printf("  Expiring in next 24h: %d\n", count)
	}
}

func processCommand(collection *mongo.Collection, command string, ip string, duration string, reason string, jsonObject map[string]interface{}) {
	switch command {
	case "add":
		handleAdd(collection, ip, duration, reason, jsonObject)
	case "del":
		handleDel(collection, ip, duration, reason, jsonObject)
	case "cleanup":
		handleCleanup(collection)
	case "list":
		handleList(collection)
	case "stats":
		handleStats(collection)
	default:
		fmt.Println("Invalid command. Available: add, del, cleanup, list, stats")
	}
}

func main() {
	config := getMongoConfig()

	// Connect to MongoDB
	client, collection, err := connectMongoDB(config)
	if err != nil {
		log.Fatal("Failed to connect to MongoDB:", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		client.Disconnect(ctx)
	}()

	if len(os.Args) < 2 {
		fmt.Println("BitNinja MongoDB Duration Manager")
		fmt.Println("\nUsage:")
		fmt.Println("  go run main.go add <ip> <duration> <reason> [json_object]")
		fmt.Println("  go run main.go del <ip> [duration] [reason] [json_object]")
		fmt.Println("  go run main.go cleanup")
		fmt.Println("  go run main.go list")
		fmt.Println("  go run main.go stats")
		fmt.Println("\nDuration examples: 24h, 30m, 2d, permanent")
		fmt.Println("\nEnvironment variables:")
		fmt.Printf("  MONGO_URI=%s\n", config.URI)
		fmt.Printf("  MONGO_DATABASE=%s\n", config.Database)
		fmt.Printf("  MONGO_COLLECTION=%s\n", config.Collection)
		fmt.Println("\nNote: Run 'cleanup' command regularly (e.g., via cron) to remove expired bans")
		os.Exit(1)
	}

	command := os.Args[1]

	// Handle special commands that don't need all parameters
	if command == "cleanup" {
		handleCleanup(collection)
		return
	}
	if command == "list" {
		handleList(collection)
		return
	}
	if command == "stats" {
		handleStats(collection)
		return
	}

	// Regular commands need at least IP
	if len(os.Args) < 3 {
		fmt.Println("Error: IP address required")
		os.Exit(1)
	}

	ip := os.Args[2]
	duration := ""
	reason := ""
	var jsonObject map[string]interface{}

	if len(os.Args) > 3 {
		duration = os.Args[3]
	}
	if len(os.Args) > 4 {
		reason = os.Args[4]
	}
	if len(os.Args) > 5 {
		jsonStr := os.Args[5]
		err := json.Unmarshal([]byte(jsonStr), &jsonObject)
		if err != nil {
			fmt.Println("Invalid JSON object:", err)
			os.Exit(1)
		}
	}

	processCommand(collection, command, ip, duration, reason, jsonObject)
}
