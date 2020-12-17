package main

import (
	"context"

	"github.com/go-redis/redis/v8"
)

var partitionKey string

func initRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
			Addr: env.RedisEndpoint,
			Password: env.RedisPwd,
	})
}

func initCounter(ctx context.Context) error {
	if env.MaxTotalAllocations == 0 {
		return nil
	}
	partitionKey = env.DynamodbTableName + ":COUNT"
	rdb := initRedis()

	_, err := rdb.Get(ctx, partitionKey).Result()
	if err != nil && err == redis.Nil {
		rdb.Set(ctx, partitionKey, 0, 0)
	}
	if err != nil {
		return err
	}
	
	return nil
}

func getCount(ctx context.Context) (uint, error) {
	if env.MaxTotalAllocations == 0 {
		return 0, nil
	}
	rdb := initRedis()
	val, err := rdb.Get(ctx, partitionKey).Uint64()
	if err != nil {
		return 0, err
	}
	return uint(val), nil
}

func reachedCounter(ctx context.Context) bool {
	if env.MaxTotalAllocations == 0 {
		return false
	}

	val, err := getCount(ctx)
	if err != nil {
		slackNotification := "REDIS GET COUNT FAILED: " + err.Error()
		sendSlackNotification("https://errors.glif.io/verifier-redis-failed", slackNotification)
		return true
	}

	return val >= env.MaxTotalAllocations
}

func incrementCounter(ctx context.Context) error {
	if env.MaxTotalAllocations == 0 {
		return nil
	}
	rdb := initRedis()

	val, err := getCount(ctx)
	if err != nil {
		return err
	}
	err = rdb.Set(ctx, partitionKey, val + 1, 0).Err()
	if err != nil {
		return err
	}
	return nil
}

func resetCounter(ctx context.Context) {
	rdb := initRedis()

	err := rdb.Set(ctx, partitionKey, 0, 0).Err()
	if err != nil {
		slackNotification := "REDIS INCREMENT COUNT FAILED: " + err.Error()
		sendSlackNotification("https://errors.glif.io/verifier-redis-failed", slackNotification)
	}
	return
}