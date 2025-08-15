package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/SeaRoll/zumi/config"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

//go:generate go run github.com/SeaRoll/interfacer/cmd -struct=cacheClient -name=Cache -file=valkey_interface.go

var ErrNil = valkey.Nil // Exported error for nil values
const DefaultTimeout = 15 * time.Minute

type cacheClient struct {
	config     config.CacheConfig
	valcli     valkey.Client
	client     valkeycompat.Cmdable
	isTeardown atomic.Bool
}

func NewCache(config config.CacheConfig) (Cache, error) {
	if !config.Enabled {
		return nil, fmt.Errorf("cache is not enabled in the configuration")
	}

	cc := &cacheClient{
		config:     config,
		isTeardown: atomic.Bool{},
	}

	err := cc.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to cache: %w", err)
	}

	cc.healthCheck()

	return cc, nil
}

func (c *cacheClient) getSentinelConfig() valkey.SentinelOption {
	if !c.config.SentinelConfig.Enabled {
		return valkey.SentinelOption{}
	}

	return valkey.SentinelOption{
		MasterSet: c.config.SentinelConfig.MasterSet,
		Password:  c.config.SentinelConfig.Password,
	}
}

func (c *cacheClient) connect() error {
	valcli, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{c.config.Host + ":" + c.config.Port},
		Password:    c.config.Password,
		Sentinel:    c.getSentinelConfig(),
	})
	if err != nil {
		return fmt.Errorf("failed to create valkey client: %w", err)
	}

	client := valkeycompat.NewAdapter(valcli)

	err = client.Ping(context.Background()).Err()
	if err != nil {
		return fmt.Errorf("failed to ping cache: %w", err)
	}

	c.valcli = valcli
	c.client = client

	return nil
}

func (c *cacheClient) healthCheck() {
	go func() {
		for {
			// Check if tearing down is requested
			if c.isTeardown.Load() {
				slog.Info("Cache is being torn down, skipping healthCheck check")
				return
			}

			err := c.client.Ping(context.Background()).Err()
			if err != nil {
				slog.Error("Cache is not healthy", "error", err)

				err := c.connect()
				if err != nil {
					slog.Error("Failed to reconnect to cache", "error", err)
				} else {
					slog.Info("Reconnected to cache")
				}
			}

			time.Sleep(5 * time.Second)
		}
	}()
}

// Publish publishes a message to a channel.
func (c *cacheClient) Publish(ctx context.Context, channel string, message string) error {
	if channel == "" {
		return errors.New("channel cannot be empty")
	}

	if message == "" {
		return errors.New("message cannot be empty")
	}

	err := c.client.Publish(ctx, channel, message).Err()
	if err != nil {
		return fmt.Errorf("failed to publish message to channel %s: %w", channel, err)
	}

	return nil
}

// Subscribe listens to a channel and calls the callback function for each message received.
// Make use of a goroutine to run this function, as it will block until the context is done.
func (c *cacheClient) Subscribe(channel string, callback func(msg string) error) error {
	ctx := context.Background()
	pubsub := c.client.Subscribe(ctx, channel)

	defer func() {
		err := pubsub.Close()
		if err != nil {
			slog.Error("Error closing pubsub", "error", err)
		}
	}()

	slog.Info("Subscribing to channel", "channel", channel)

	for {
		msg, err := pubsub.ReceiveMessage(ctx)
		if err != nil {
			slog.Error("Error receiving message", "error", err)
			time.Sleep(1 * time.Second)

			continue
		}

		if msg.Channel != channel {
			slog.Warn("Received message from unexpected channel", "expected", channel, "received", msg.Channel)
			continue
		}

		if msg.Payload == "" {
			slog.Warn("Received empty message")
			continue
		}

		err = callback(msg.Payload)
		if err != nil {
			slog.Error("Error processing message", "message", msg.Payload, "error", err)
			continue
		}
	}
}

// Lock locks a key in the cache for a specified duration.
func (c *cacheClient) Lock(ctx context.Context, key string, timeout time.Duration) (bool, error) {
	success, err := c.client.SetNX(ctx, key, "1", timeout).Result()
	if err != nil {
		return false, fmt.Errorf("failed to lock key %s: %w", key, err)
	}

	return success, nil
}

// Unlock unlocks a key in the cache.
func (c *cacheClient) Unlock(ctx context.Context, key string) error {
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to unlock key %s: %w", key, err)
	}

	return nil
}

// TTL returns the time to live of a key in the cache.
func (c *cacheClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	duration, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get TTL for key %s: %w", key, err)
	}

	return duration, nil
}

// Disconnect forcefully disconnects the cache client.
// if `noTeardown` is true, it will not set the teardown flag,
// allowing the client to be reused later.
//
// If `noTeardown` is false or not provided, it will set the teardown flag
// and close the client connection, preventing any further operations.
// This is useful for graceful shutdowns or when you want to ensure the client
// is no longer usable after disconnecting.
func (c *cacheClient) Disconnect(noTeardown ...bool) {
	if c.client == nil || c.valcli == nil {
		return
	}

	if len(noTeardown) == 0 || !noTeardown[0] {
		c.isTeardown.Store(true)
	}

	c.valcli.Close()
	slog.Info("Disconnected from valkey")
}

// Set sets a value in the cache with a specified timeout.
func (c *cacheClient) Set(ctx context.Context, key string, value any, timeout time.Duration) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	jsonValueString := string(jsonValue)

	err = c.client.Set(ctx, key, jsonValueString, timeout).Err()
	if err != nil {
		return fmt.Errorf("failed to set value in cache: %w", err)
	}

	return nil
}

// IncrBy increments the value of a key in the cache by a specified amount.
func (c *cacheClient) IncrBy(ctx context.Context, key string, increment int64) (int64, error) {
	res, err := c.client.IncrBy(ctx, key, increment).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to increment key %s by %d: %w", key, increment, err)
	}

	return res, nil
}

// Get retrieves a value from the cache by its key and unmarshals it into the provided value.
func (c *cacheClient) Get(ctx context.Context, key string, value any) error {
	result, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to get value from cache: %w", err)
	}

	err = json.Unmarshal([]byte(result), value)
	if err != nil {
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return nil
}

// GetWithResetTTL retrieves a value from the cache by its key, unmarshals it into the provided value,.
func (c *cacheClient) GetWithResetTTL(ctx context.Context, key string, value any, ttl time.Duration) error {
	err := c.Get(ctx, key, value)
	if err != nil {
		return fmt.Errorf("failed to get value from cache: %w", err)
	}

	err = c.client.Expire(ctx, key, ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to reset TTL: %w", err)
	}

	return nil
}

// Exists checks if a key exists.
func (c *cacheClient) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check if key exists: %w", err)
	}

	return result == 1, nil
}

// Delete removes a key from the cache.
func (c *cacheClient) Delete(ctx context.Context, key string) error {
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key from cache: %w", err)
	}

	return nil
}

// Wrapped will attempt to get the value from the cache, if it doesn't exist it will call the fallbackFunc
// and set the value in the cache. Timeout is optional and will default to 15 minutes. 0 means no timeout.
// -1 means no cache.
func (c *cacheClient) Wrapped(ctx context.Context, key string, data any, fallbackFunc func() error, timeout ...time.Duration) error {
	// if timeout is -1, don't use cache
	if len(timeout) > 0 && timeout[0] == -1 {
		return fallbackFunc()
	}

	err := c.Get(ctx, key, data)
	if err == nil {
		return nil
	}

	err = fallbackFunc()
	if err != nil {
		return fmt.Errorf("fallback function failed: %w", err)
	}

	to := DefaultTimeout
	if len(timeout) > 0 {
		to = timeout[0]
	}

	err = c.Set(ctx, key, data, to)
	if err != nil {
		return fmt.Errorf("failed to set value in cache: %w", err)
	}

	return nil
}
