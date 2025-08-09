package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Simple representation to check data structure
type CharacterData struct {
	ClassResources json.RawMessage `json:"class_resources"`
}

func main() {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatal("Failed to parse Redis URL:", err)
	}

	client := redis.NewClient(opt)
	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}

	fmt.Println("Connected to Redis:", redisURL)
	fmt.Println("Scanning for corrupted character data...")

	// Find all character keys
	iter := client.Scan(ctx, 0, "character:*", 0).Iterator()

	var corruptedKeys []string
	var checkedCount int

	for iter.Next(ctx) {
		key := iter.Val()
		checkedCount++

		// Get the data
		data, err := client.Get(ctx, key).Result()
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", key, err)
			continue
		}

		// Try to parse it
		var charData CharacterData
		if err := json.Unmarshal([]byte(data), &charData); err != nil {
			fmt.Printf("✗ Corrupted JSON in %s\n", key)
			corruptedKeys = append(corruptedKeys, key)
			continue
		}

		// Check if class_resources is a number (old format) instead of object/string
		if charData.ClassResources != nil {
			resourceStr := string(charData.ClassResources)
			// If it's just a number, it's the old format
			if !strings.Contains(resourceStr, "{") && !strings.Contains(resourceStr, "\"") {
				fmt.Printf("✗ Old format detected in %s: class_resources is %s\n", key, resourceStr)
				corruptedKeys = append(corruptedKeys, key)
			}
		}
	}

	if err := iter.Err(); err != nil {
		log.Fatal("Error during scan:", err)
	}

	fmt.Printf("\nChecked %d keys, found %d corrupted entries\n", checkedCount, len(corruptedKeys))

	if len(corruptedKeys) == 0 {
		fmt.Println("No corrupted data found!")
		return
	}

	fmt.Println("\nCorrupted keys:")
	for _, key := range corruptedKeys {
		fmt.Printf("  - %s\n", key)
	}

	// Ask for confirmation before deletion
	fmt.Print("\nDo you want to DELETE these corrupted entries? (yes/no): ")
	var response string
	fmt.Scanln(&response)

	if response == "yes" {
		for _, key := range corruptedKeys {
			if err := client.Del(ctx, key).Err(); err != nil {
				fmt.Printf("Failed to delete %s: %v\n", key, err)
			} else {
				fmt.Printf("Deleted %s\n", key)
			}
		}
		fmt.Println("\nCleanup complete!")
	} else {
		fmt.Println("Aborted - no changes made")
	}
}
