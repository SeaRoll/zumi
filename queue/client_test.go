package queue

import (
	"context"
	"testing"
	"time"

	"github.com/SeaRoll/zumi/config"
	"github.com/stretchr/testify/assert"
)

const configYaml = `
queue:
  enabled: true
  url: nats://localhost:4222
  name: default
  prefix: events
  maxAge: 24h
`

func setupQueue(ctx context.Context, t *testing.T) Queue {
	t.Helper()

	cfg, err := config.FromYAML[config.BaseConfig](configYaml)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	t.Logf("Config: %+v", cfg)

	queue, err := NewQueue(cfg.GetBaseConfig().Queue)
	if err != nil {
		t.Fatalf("Failed to create queue client: %v", err)
	}

	return queue
}

// publish and subscribe a message
func TestPublishSubscribe(t *testing.T) {
	ctx := context.Background()
	queue := setupQueue(ctx, t)

	var receivedMessage []byte

	go queue.Consume(ConsumerConfig{
		ConsumerName: "api",
		Topic:        "events.test",
		FetchLimit:   1,
		Callback: func(ctx context.Context, events []Event) []int {
			if len(events) == 0 {
				return []int{}
			}
			receivedMessage = events[0].Payload
			return []int{events[0].Index}
		},
	})

	err := queue.Publish("events.test", []byte("test message"))
	if err != nil {
		t.Fatalf("Failed to publish message: %v", err)
	}

	assert.Eventually(
		t,
		func() bool {
			return receivedMessage != nil
		},
		time.Duration(5*time.Second),
		time.Duration(100*time.Millisecond),
		"Message was not received in time",
	)

	assert.Equal(t, []byte("test message"), receivedMessage)
}
