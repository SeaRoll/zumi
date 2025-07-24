package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/SeaRoll/zumi/cache"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestSetGetValues(t *testing.T) {
	ctx := context.Background()
	cache, err := cache.NewCache(cache.CacheConfig{
		Host:           "localhost",
		Port:           "6379",
		Password:       "",
		SentinelConfig: nil,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Disconnect()

	tests := []struct {
		key           string        // key of the cache entry
		value         any           // value to set in the cache
		timeout       time.Duration // timeout for the cache entry
		waitFor       time.Duration // wait until confirming the value is set
		expected      any           // expected value to be retrieved from the cache
		expectedError bool          // whether an error is expected when retrieving the value (probably due to timeout)
	}{
		{ // normal set string value
			key:      uuid.NewString(),
			value:    "testValue1",
			timeout:  5 * time.Second,
			waitFor:  time.Second,
			expected: "testValue1",
		},
		{ // normal set int value
			key:      uuid.NewString(),
			value:    42,
			timeout:  5 * time.Second,
			waitFor:  time.Second,
			expected: float64(42), // Valkey stores numbers as float64
		},
		{ // normal set struct value
			key:      uuid.NewString(),
			value:    map[string]any{"key1": 1, "key2": 2},
			timeout:  5 * time.Second,
			waitFor:  time.Second,
			expected: map[string]any{"key1": float64(1), "key2": float64(2)},
		},
		{ // set value with a timeout error, expect error after timeout
			key:           uuid.NewString(),
			value:         "longTimeoutValue",
			timeout:       time.Millisecond,
			waitFor:       1 * time.Second, // wait longer than the timeout
			expected:      nil,             // expect nil because we will wait longer than the timeout
			expectedError: true,            // expect an error because we will try to get the value after the timeout
		},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if err := cache.Set(ctx, tt.key, tt.value, tt.timeout); err != nil {
				t.Fatalf("Failed to set value: %v", err)
			}

			// Wait until the specified time before checking the value
			time.Sleep(tt.waitFor)

			var value any
			err := cache.Get(ctx, tt.key, &value)
			if tt.expectedError {
				assert.Error(t, err, "Expected error when retrieving value after timeout")
				t.Logf("Received value: %v", value)
				return
			} else {
				assert.NoError(t, err, "Unexpected error when retrieving value")
				assert.Equal(t, tt.expected, value, "Expected value does not match retrieved value")
			}
		})
	}
}

func TestExists(t *testing.T) {
	ctx := context.Background()
	cache, err := cache.NewCache(cache.CacheConfig{
		Host:           "localhost",
		Port:           "6379",
		Password:       "",
		SentinelConfig: nil,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Disconnect()

	// add a key to the cache
	key := uuid.NewString()
	value := "testValue"
	err = cache.Set(ctx, key, value, 1*time.Second)
	assert.NoError(t, err, "Unexpected error setting value in cache")

	exists, err := cache.Exists(ctx, key)
	assert.NoError(t, err, "Unexpected error checking if key exists")
	assert.True(t, exists, "Expected key to exist in cache")

	// wait for the key to expire
	time.Sleep(1 * time.Second)

	exists, err = cache.Exists(ctx, key)
	assert.NoError(t, err, "Unexpected error checking if key exists after expiration")
	assert.False(t, exists, "Expected key to not exist in cache after expiration")
}

func TestDeleteKey(t *testing.T) {
	ctx := context.Background()
	cache, err := cache.NewCache(cache.CacheConfig{
		Host:           "localhost",
		Port:           "6379",
		Password:       "",
		SentinelConfig: nil,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Disconnect()

	// add a key to the cache
	key := uuid.NewString()
	value := "testValue"
	err = cache.Set(ctx, key, value, 1*time.Second)
	assert.NoError(t, err, "Unexpected error setting value in cache")

	// delete the key
	err = cache.Delete(ctx, key)
	assert.NoError(t, err, "Unexpected error deleting key from cache")

	// check if the key exists
	exists, err := cache.Exists(ctx, key)
	assert.NoError(t, err, "Unexpected error checking if key exists after deletion")
	assert.False(t, exists, "Expected key to not exist in cache after deletion")
}

func TestWrapped(t *testing.T) {
	ctx := context.Background()
	cache, err := cache.NewCache(cache.CacheConfig{
		Host:           "localhost",
		Port:           "6379",
		Password:       "",
		SentinelConfig: nil,
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer cache.Disconnect()

	// call the wrapped function two times with timeout as -1, it should fallback each time
	key := uuid.NewString()
	timesCalled := 0
	var anyData string
	for range 2 {
		if err := cache.Wrapped(ctx, key, &anyData, func() error {
			timesCalled++
			anyData = "fallbackValue"
			return nil
		}, -1); err != nil {
			t.Fatalf("Unexpected error in wrapped function: %v", err)
		}
	}
	assert.Equal(t, 2, timesCalled, "Expected fallback function to be called twice")

	// call the wrapped function with a timeout, it should only call the fallback function once
	key = uuid.NewString()
	timesCalled = 0
	for range 2 {
		if err := cache.Wrapped(ctx, key, &anyData, func() error {
			timesCalled++
			anyData = "fallbackValue"
			return nil
		}); err != nil {
			t.Fatalf("Unexpected error in wrapped function: %v", err)
		}
	}
	assert.Equal(t, 1, timesCalled, "Expected fallback function to be called only once")
}
