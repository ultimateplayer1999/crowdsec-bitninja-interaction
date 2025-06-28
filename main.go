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
	RemovedAt *time.Time             `bson:"removed_at,omitempty" json:"removed_at,omitempty"`
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
		log.Printf("Warning: Could not load .env file: %v", err)
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
	// Increase connection timeout significantly
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Configure client options with more robust settings
	clientOptions := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(10).
		SetMinPoolSize(1).
		SetMaxConnIdleTime(30 * time.Second).
		SetConnectTimeout(10 * time.Second).
		SetSocketTimeout(30 * time.Second).
		SetServerSelectionTimeout(10 * time.Second)

	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Test connection with retry logic
	var pingErr error
	for i := 0; i < 3; i++ {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr = client.Ping(pingCtx, nil)
		pingCancel()

		if pingErr == nil {
			break
		}

		if i < 2 {
			log.Printf("Ping attempt %d failed, retrying: %v", i+1, pingErr)
			time.Sleep(2 * time.Second)
		}
	}

	if pingErr != nil {
		client.Disconnect(context.Background())
		return nil, nil, fmt.Errorf("failed to ping MongoDB after retries: %v", pingErr)
	}

	collection := client.Database(config.Database).Collection(config.Collection)

	// Create indexes with longer timeout and better error handling
	err = createIndexes(collection)
	if err != nil {
		log.Printf("Warning: Could not create indexes: %v", err)
		// Don't fail completely, just warn
	}

	return client, collection, nil
}

func createIndexes(collection *mongo.Collection) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "ip", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys: bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys: bson.D{{Key: "status", Value: 1}},
			Options: options.Index().SetBackground(true),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "expires_at", Value: 1},
			},
			Options: options.Index().SetBackground(true),
		},
	}

	// Use longer timeout for index creation
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexes)
	return err
}

func parseDuration(duration string) (time.Duration, error) {
	if duration == "permanent" || duration == "0" {
		return 0, nil // 0 means permanent
	}

	// Check if it's just a number (assume seconds)
	if seconds, err := strconv.Atoi(duration); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}

	// Handle days suffix (not supported by standard time.ParseDuration)
	if strings.HasSuffix(duration, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(duration, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	// Handle other suffixes with explicit seconds support
	if strings.HasSuffix(duration, "s") {
		// Let time.ParseDuration handle it
		return time.ParseDuration(duration)
	}

	// For other formats, try standard Go duration parsing
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

	// Increase timeout for insert operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = collection.InsertOne(ctx, record)
	return err
}

func removeBanRecord(collection *mongo.Collection, ip string) error {
	// Increase timeout for update operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	filter := bson.M{
		"ip":     ip,
		"status": "active",
	}

	update := bson.M{
		"$set": bson.M{
			"status":     "removed",
			"removed_at": now,
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	return err
}

func cleanupExpiredBans(collection *mongo.Collection) error {
	// Significantly increase timeout for cleanup operations
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Process in batches to avoid timeout
	batchSize := 100
	processedCount := 0

	for {
		// Find expired bans in batches
		filter := bson.M{
			"expires_at": bson.M{"$lte": time.Now()},
			"status":     "active",
		}

		opts := options.Find().SetLimit(int64(batchSize))
		cursor, err := collection.Find(ctx, filter, opts)
		if err != nil {
			return fmt.Errorf("error finding expired bans: %v", err)
		}

		var expiredBans []BanRecord
		if err = cursor.All(ctx, &expiredBans); err != nil {
			cursor.Close(ctx)
			return fmt.Errorf("error decoding expired bans: %v", err)
		}
		cursor.Close(ctx)

		if len(expiredBans) == 0 {
			break // No more expired bans
		}

		// Process this batch
		for _, ban := range expiredBans {
			fmt.Printf("Removing expired ban for IP: %s (banned at: %s)\n",
				ban.IP, ban.BannedAt.Format("2006-01-02 15:04:05"))

			// Remove from BitNinja with timeout
			cmd := exec.Command("bitninjacli", "--blacklist", fmt.Sprintf("--del=%s", ban.IP))
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			// Set timeout for external command
			cmdCtx, cmdCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cmdCancel()

			if err := cmd.Start(); err != nil {
				fmt.Printf("Error starting command for IP %s: %v\n", ban.IP, err)
				continue
			}

			// Wait for command to complete or timeout
			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case err := <-done:
				if err != nil {
					fmt.Printf("Error removing IP %s from BitNinja: %v\n", ban.IP, err)
					continue
				}
			case <-cmdCtx.Done():
				fmt.Printf("Timeout removing IP %s from BitNinja\n", ban.IP)
				if cmd.Process != nil {
					cmd.Process.Kill()
				}
				continue
			}

			// Mark as expired in MongoDB
			updateFilter := bson.M{"_id": ban.ID}
			update := bson.M{
				"$set": bson.M{
					"status": "expired",
				},
			}

			updateCtx, updateCancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, err = collection.UpdateOne(updateCtx, updateFilter, update)
			updateCancel()

			if err != nil {
				fmt.Printf("Error updating MongoDB for IP %s: %v\n", ban.IP, err)
			}
		}

		processedCount += len(expiredBans)

		// Check if we processed less than batch size (last batch)
		if len(expiredBans) < batchSize {
			break
		}
	}

	fmt.Printf("Processed %d expired bans\n", processedCount)
	return nil
}

func purgeOldRecords(collection *mongo.Collection, olderThanDays int) error {
	// Increase timeout for purge operations
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Calculate cutoff date
	cutoffDate := time.Now().AddDate(0, 0, -olderThanDays)

	filter := bson.M{
		"$or": []bson.M{
			{
				"status": "removed",
				"removed_at": bson.M{"$lt": cutoffDate},
			},
			{
				"status": "expired",
				"banned_at": bson.M{"$lt": cutoffDate},
			},
		},
	}

	// Count records to purge
	count, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return fmt.Errorf("error counting records to purge: %v", err)
	}

	if count == 0 {
		fmt.Printf("No records found older than %d days to purge\n", olderThanDays)
		return nil
	}

	fmt.Printf("Found %d records older than %d days to purge\n", count, olderThanDays)

	// Process in batches if there are many records
	if count > 1000 {
		fmt.Println("Large number of records detected, processing in batches...")
		return purgeInBatches(collection, filter, olderThanDays)
	}

	// Log records that will be purged (limit to avoid timeout)
	opts := options.Find().SetLimit(100)
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return fmt.Errorf("error finding records to purge: %v", err)
	}
	defer cursor.Close(ctx)

	var recordsToPurge []BanRecord
	if err = cursor.All(ctx, &recordsToPurge); err != nil {
		return fmt.Errorf("error decoding records to purge: %v", err)
	}

	fmt.Println("Sample of records to be purged:")
	for i, record := range recordsToPurge {
		if i >= 10 { // Limit output
			fmt.Printf("... and %d more records\n", len(recordsToPurge)-10)
			break
		}

		var dateStr string
		if record.RemovedAt != nil {
			dateStr = record.RemovedAt.Format("2006-01-02 15:04:05")
		} else {
			dateStr = record.BannedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("  IP: %s, Status: %s, Date: %s, Reason: %s\n",
			record.IP, record.Status, dateStr, record.Reason)
	}

	// Actually delete
	result, err := collection.DeleteMany(ctx, filter)
	if err != nil {
		return fmt.Errorf("error purging records: %v", err)
	}

	fmt.Printf("Successfully purged %d records from database\n", result.DeletedCount)
	return nil
}

func purgeInBatches(collection *mongo.Collection, filter bson.M, olderThanDays int) error {
	batchSize := 1000
	totalDeleted := int64(0)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		// Delete in batches
		opts := options.Delete().SetHint(bson.D{{Key: "status", Value: 1}})
		result, err := collection.DeleteMany(ctx, filter, opts)
		cancel()

		if err != nil {
			return fmt.Errorf("error purging batch: %v", err)
		}

		totalDeleted += result.DeletedCount
		fmt.Printf("Purged batch: %d records\n", result.DeletedCount)

		// If we deleted less than batch size, we're done
		if result.DeletedCount < int64(batchSize) {
			break
		}

		// Small delay between batches
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("Successfully purged %d total records from database\n", totalDeleted)
	return nil
}

func handleAdd(collection *mongo.Collection, ip string, duration string, reason string, jsonObject map[string]interface{}) {
	// Add to BitNinja blacklist
	enhancedReason := fmt.Sprintf("%s (Duration: %s, Host: %s)", reason, duration, getHostname())
	cmd := exec.Command("bitninjacli", "--blacklist", fmt.Sprintf("--add=%s", ip), fmt.Sprintf("--comment=%s", enhancedReason))

	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("Error adding IP to BitNinja: %v\n", err)
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
		fmt.Printf("Error deleting IP from BitNinja: %v\n", err)
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

func handlePurge(collection *mongo.Collection, daysStr string) {
	days := 30 // default
	if daysStr != "" {
		var err error
		days, err = strconv.Atoi(daysStr)
		if err != nil {
			fmt.Printf("Invalid number of days: %s, using default of 30 days\n", daysStr)
			days = 30
		}
	}

	fmt.Printf("Purging records older than %d days...\n", days)
	err := purgeOldRecords(collection, days)
	if err != nil {
		fmt.Printf("Error during purge: %v\n", err)
	} else {
		fmt.Println("Purge completed")
	}
}

func handleList(collection *mongo.Collection) {
	// Increase timeout for list operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First get the accurate total count
	filter := bson.M{"status": "active"}
	totalCount, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		fmt.Printf("Error counting active bans: %v\n", err)
		return
	}

	// Find active bans, sorted by banned_at descending (limited for display)
	opts := options.Find().
		SetSort(bson.D{{Key: "banned_at", Value: -1}}).
		SetLimit(1000) // Limit results to avoid timeout

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

	fmt.Printf("\nTotal active bans: %d\n", totalCount)
	if int64(len(bans)) < totalCount {
		fmt.Printf("Showing latest %d entries (sorted by banned date)\n", len(bans))
	}
}

func handleStats(collection *mongo.Collection) {
	// Increase timeout for stats operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	// Count records that can be purged (older than 30 days)
	cutoffDate := time.Now().AddDate(0, 0, -30)
	purgeableFilter := bson.M{
		"$or": []bson.M{
			{
				"status": "removed",
				"removed_at": bson.M{"$lt": cutoffDate},
			},
			{
				"status": "expired",
				"banned_at": bson.M{"$lt": cutoffDate},
			},
		},
	}

	purgeableCount, err := collection.CountDocuments(ctx, purgeableFilter)
	if err == nil {
		fmt.Printf("  Purgeable (>30 days old): %d\n", purgeableCount)
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
	case "purge":
		handlePurge(collection, duration) // duration wordt hergebruikt als days parameter
	case "list":
		handleList(collection)
	case "stats":
		handleStats(collection)
	default:
		fmt.Println("Invalid command. Available: add, del, cleanup, purge, list, stats")
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			log.Printf("Error disconnecting from MongoDB: %v", err)
		}
	}()

	if len(os.Args) < 2 {
		fmt.Println("BitNinja MongoDB Duration Manager")
		fmt.Println("\nUsage:")
		fmt.Println("  go run main.go add <ip> <duration> <reason> [json_object]")
		fmt.Println("  go run main.go del <ip> [duration] [reason] [json_object]")
		fmt.Println("  go run main.go cleanup")
		fmt.Println("  go run main.go purge [days]           # Default: 30 days")
		fmt.Println("  go run main.go list")
		fmt.Println("  go run main.go stats")
		fmt.Println("\nDuration examples: 24h, 30m, 2d, permanent")
		fmt.Println("\nEnvironment variables:")
		fmt.Printf("  MONGO_URI=%s\n", config.URI)
		fmt.Printf("  MONGO_DATABASE=%s\n", config.Database)
		fmt.Printf("  MONGO_COLLECTION=%s\n", config.Collection)
		fmt.Println("\nNote: Run 'cleanup' and 'purge' commands regularly (e.g., via cron)")
		fmt.Println("      - cleanup: removes expired active bans")
		fmt.Println("      - purge: permanently deletes old removed/expired records")
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
	if command == "purge" {
		days := ""
		if len(os.Args) > 2 {
			days = os.Args[2]
		}
		handlePurge(collection, days)
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
