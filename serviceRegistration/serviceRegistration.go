package serviceRegistration

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog/log"
)

// GetLocalIP fetches the local IP address of the machine
// If BIND_IP environment variable is set, it returns that value instead
func GetLocalIP() (string, error) {
	if bindIP := os.Getenv("BIND_IP"); bindIP != "" {
		return bindIP, nil
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iFace := range interfaces {
		if iFace.Flags&net.FlagUp == 0 || iFace.Flags&net.FlagLoopback != 0 {
			continue // interface is down or loopback
		}

		addresses, err := iFace.Addrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addresses {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.IsGlobalUnicast() {
					return v.IP.String(), nil
				}
			}
		}
	}
	return "", fmt.Errorf("no IP address found")
}

// saveIPToRedis saves the IP address to Redis every 60 seconds with an expiration of 100 seconds
func saveIPToRedis(redisClient *redis.Client) {
	var ctx = context.Background()
	for {
		ip, err := GetLocalIP()
		if err != nil {
			fmt.Println("Error fetching IP address:", err)
			time.Sleep(60 * time.Second)
			continue
		}

		key := fmt.Sprintf("voice.bot.ip.%s", ip)
		log.Info().Msgf("Saved key: %s", key)
		err = redisClient.Set(ctx, key, ip, 100*time.Second).Err()
		if err != nil {
			fmt.Println("Error saving to Redis:", err)
		} else {
			fmt.Printf("Saved IP %s to Redis with key %s\n", ip, key)
		}

		time.Sleep(60 * time.Second)
	}
}

func StartServiceDiscoverySignal() {

	redisURL := os.Getenv("REDIS_URL")

	// Handle different Redis URL formats
	redisURL = strings.TrimPrefix(redisURL, "redis://")

	// If no port is specified, add the default port 6379
	if !strings.Contains(redisURL, ":") {
		redisURL = redisURL + ":6379"
	}

	log.Info().Msgf("REDIS_URL: %s", redisURL)
	if redisURL == "" {
		fmt.Println("REDIS_URL environment variable is not set")
		return
	}

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisURL, // Use the processed REDIS_URL from environment
	})

	defer redisClient.Close()

	// Start saving IP to Redis
	saveIPToRedis(redisClient)
}
