package redisclient

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"sms_service/config"
)

func NewClient(cfg *config.Config) *redis.Client {
	addr := fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort)
	log.Printf("[REDIS] Connecting | addr=%s", addr)

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: cfg.RedisPassword,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("[REDIS] Failed to connect | addr=%s | error=%v", addr, err)
	}

	log.Printf("[REDIS] Connected and ready | addr=%s", addr)
	return client
}
