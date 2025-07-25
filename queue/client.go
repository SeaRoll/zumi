package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/failsafe-go/failsafe-go"
	"github.com/failsafe-go/failsafe-go/retrypolicy"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

//go:generate go run github.com/SeaRoll/interfacer/cmd -struct=pubsubClient -name=Queue

type pubsubClient struct {
	js          jetstream.JetStream
	stream      jetstream.Stream
	retryPolicy retrypolicy.RetryPolicy[any]
}

type NewPubsubClientParams struct {
	ConnectionUrl string        // NATS server connection URL
	Name          string        // Name of the JetStream stream
	TopicPrefix   string        // Prefix for topics in the stream
	MaxAge        time.Duration // Maximum age of messages in the stream
}

// Initializes a new Pubsub Client
func NewPubsubClient(params NewPubsubClientParams) (Queue, error) {
	nc, err := nats.Connect(
		params.ConnectionUrl,
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Error("Disconnected from NATS server", "error", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			slog.Info("Reconnected to NATS server", "url", c.ConnectedUrl())
		}),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			slog.Error("NATS error", "error", err, "subscription", sub.Subject)
		}),
		nats.MaxReconnects(-1), // unlimited reconnect attempts
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS server: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream client: %w", err)
	}

	stream, err := js.CreateOrUpdateStream(context.Background(), jetstream.StreamConfig{
		Name:      params.Name,
		Subjects:  []string{params.TopicPrefix + ".>"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    params.MaxAge,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create or update stream: %w", err)
	}

	retryPolicy := retrypolicy.Builder[any]().
		WithBackoff(time.Second, 30*time.Second).
		WithMaxRetries(5).
		Build()

	return &pubsubClient{
		js:          js,
		stream:      stream,
		retryPolicy: retryPolicy,
	}, nil
}

// Publishes a message to the specified topic.
// The function accepts a variadic parameter for timeout duration, defaulting to 5 seconds if not provided.
func (p *pubsubClient) Publish(topic string, message []byte, timeout ...time.Duration) error {
	return failsafe.Run(func() error {
		defaultTimeout := 5 * time.Second
		if len(timeout) > 0 {
			defaultTimeout = timeout[0]
		}
		// timeout with 5 seconds
		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		_, err := p.js.Publish(ctx, topic, message)
		if err != nil {
			return fmt.Errorf("failed to publish message to topic %s: %w", topic, err)
		}
		return nil
	}, p.retryPolicy)
}

type Event struct {
	Index   int    // the index of the event in the batch
	Payload []byte // the data of the event
}

type ackFunc func() error

// CallbackFunc is a function that is called when a message is received
// This will not be called when there are no messages to process
// It receives a context and a slice of events, and returns a slice of integers
// representing the indices of the events that were successfully processed.
type CallbackFunc func(ctx context.Context, events []Event) []int

// Configuration for a consumer
type ConsumerConfig struct {
	ConsumerName    string         // The name for the consumer
	Subject         string         // The subject to listen on
	FetchLimit      int            // The maximum number of messages to fetch per second
	Callback        CallbackFunc   // The callback function to process messages
	CallbackTimeout *time.Duration // Optional timeout for the callback function, defaults to 1 minute
	Wait            *time.Duration // Optional wait time for the consumer before fetching messages, defaults to 1 second
}

func (p *pubsubClient) getFetchWaitAndCallbackTimeout(config ConsumerConfig) (time.Duration, time.Duration) {
	fetchWait := time.Second // default
	if config.Wait != nil {
		fetchWait = *config.Wait
	}

	callbackTimeout := time.Minute // default
	if config.CallbackTimeout != nil {
		callbackTimeout = *config.CallbackTimeout
	}

	return fetchWait, callbackTimeout
}

// Runs a consumer by given configuration and callback function
// OBS: This function is blocking, so make sure to run it in a goroutine if
// you want to run other code in parallel.
// Returns an error if the consumer could not be created or updated.
func (p *pubsubClient) Consume(config ConsumerConfig) error {
	cons, err := p.stream.CreateOrUpdateConsumer(
		context.Background(),
		jetstream.ConsumerConfig{
			Name:          config.ConsumerName,
			Durable:       config.ConsumerName,
			FilterSubject: config.Subject,
		})
	if err != nil {
		return fmt.Errorf("failed to create or update consumer: %w", err)
	}

	fetchWait, callbackTimeout := p.getFetchWaitAndCallbackTimeout(config)

	for {
		msgs, err := p.fetchMessages(cons, config, fetchWait)
		if err != nil {
			continue
		}

		events := []Event{}
		acks := []ackFunc{}
		for msg := range msgs.Messages() {
			if err := msg.InProgress(); err != nil {
				slog.Error("Error setting message in progress", "error", err)
				break
			}
			events = append(events, Event{
				Index:   len(events),
				Payload: msg.Data(),
			})
			acks = append(acks, msg.Ack)
		}

		if len(events) == 0 {
			continue
		}

		res := p.performCallback(config.Callback, events, callbackTimeout)
		p.ackSuccessfulMsgs(res, config, acks)
	}
}

func (p *pubsubClient) ackSuccessfulMsgs(res []int, config ConsumerConfig, acks []ackFunc) {
	if len(res) == 0 {
		return
	}

	for _, idx := range res {
		if idx < 0 || idx >= len(acks) {
			continue
		}
		if err := failsafe.Run(func() error {
			return acks[idx]()
		}, p.retryPolicy); err != nil {
			slog.Error(
				"Failed to acknowledge message",
				"error",
				err,
				"index",
				idx,
				"consumer",
				config.ConsumerName,
				"subject",
				config.Subject,
			)
		}
	}
}

func (p *pubsubClient) fetchMessages(
	cons jetstream.Consumer,
	config ConsumerConfig,
	fetchWait time.Duration,
) (jetstream.MessageBatch, error) {
	var msgs jetstream.MessageBatch
	if err := failsafe.Run(func() error {
		var err error
		msgs, err = cons.Fetch(config.FetchLimit, jetstream.FetchMaxWait(fetchWait))
		return fmt.Errorf(
			"failed to fetch messages for consumer %s on subject %s: %w",
			config.ConsumerName,
			config.Subject,
			err,
		)
	}, p.retryPolicy); err != nil {
		return nil, fmt.Errorf(
			"failed to fetch messages for consumer %s on subject %s: %w",
			config.ConsumerName,
			config.Subject,
			err,
		)
	}
	return msgs, nil
}

func (p *pubsubClient) performCallback(callbackFn CallbackFunc, events []Event, callbackTimeout time.Duration) []int {
	ctx, cancel := context.WithTimeout(context.Background(), callbackTimeout)
	defer cancel()
	return callbackFn(ctx, events)
}
