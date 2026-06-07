package event

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

const (
	AuthStreamName    = "AUTH_EVENTS"
	AuthStreamSubject = "auth.>"
	ConsumerName      = "user-service"

	SubjectUserCreated = "auth.user.created"
)

// UserCreatedPayload is the event structure published by auth-service.
type UserCreatedPayload struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
}

// ProfileCreator is the interface the consumer uses to create profiles.
// Decouples the consumer from the service layer.
type ProfileCreator interface {
	CreateInitialProfile(ctx context.Context, userID uuid.UUID, email string) error
}

type Consumer struct {
	js      jetstream.JetStream
	creator ProfileCreator
	logger  *zap.Logger
	cancel  context.CancelFunc
}

func NewConsumer(nc *nats.Conn, creator ProfileCreator, logger *zap.Logger) (*Consumer, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("creating jetstream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ensure the AUTH_EVENTS stream exists.
	// The auth-service creates it, but we ensure it here for resilience.
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      AuthStreamName,
		Subjects:  []string{AuthStreamSubject},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
	})
	if err != nil {
		return nil, fmt.Errorf("ensuring stream %s: %w", AuthStreamName, err)
	}

	return &Consumer{
		js:      js,
		creator: creator,
		logger:  logger,
	}, nil
}

// Start begins consuming events. Call Stop() to shut down gracefully.
func (c *Consumer) Start(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)

	consumer, err := c.js.CreateOrUpdateConsumer(ctx, AuthStreamName, jetstream.ConsumerConfig{
		Durable:       ConsumerName,
		FilterSubject: SubjectUserCreated,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		MaxDeliver:    5,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("creating consumer: %w", err)
	}

	c.logger.Info("NATS consumer started",
		zap.String("stream", AuthStreamName),
		zap.String("subject", SubjectUserCreated),
	)

	go c.consume(ctx, consumer)
	return nil
}

func (c *Consumer) consume(ctx context.Context, consumer jetstream.Consumer) {
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("NATS consumer stopping")
			return
		default:
		}

		msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.Warn("fetch error, retrying", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		for msg := range msgs.Messages() {
			c.handleMessage(ctx, msg)
		}
	}
}

func (c *Consumer) handleMessage(ctx context.Context, msg jetstream.Msg) {
	ctx, span := otel.Tracer("user-service").Start(ctx, "nats.consume."+msg.Subject())
	defer span.End()

	var env Envelope
	if err := json.Unmarshal(msg.Data(), &env); err != nil {
		c.logger.Error("failed to unmarshal envelope", zap.Error(err))
		// Terminal failure — don't redeliver malformed messages.
		_ = msg.Term()
		return
	}

	switch msg.Subject() {
	case SubjectUserCreated:
		c.handleUserCreated(ctx, env, msg)
	default:
		c.logger.Warn("unhandled subject", zap.String("subject", msg.Subject()))
		_ = msg.Ack()
	}
}

func (c *Consumer) handleUserCreated(ctx context.Context, env Envelope, msg jetstream.Msg) {
	payloadBytes, err := json.Marshal(env.Payload)
	if err != nil {
		c.logger.Error("failed to re-marshal payload", zap.Error(err))
		_ = msg.Term()
		return
	}

	var payload UserCreatedPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		c.logger.Error("failed to unmarshal UserCreatedPayload", zap.Error(err))
		_ = msg.Term()
		return
	}

	c.logger.Info("handling user.created",
		zap.String("user_id", payload.UserID.String()),
		zap.String("email", maskEmail(payload.Email)),
	)

	if err := c.creator.CreateInitialProfile(ctx, payload.UserID, payload.Email); err != nil {
		c.logger.Error("failed to create initial profile",
			zap.String("user_id", payload.UserID.String()),
			zap.Error(err),
		)
		// NAK for redelivery — transient failures (DB down, etc.)
		_ = msg.Nak()
		return
	}

	_ = msg.Ack()
	c.logger.Info("profile created from user.created event",
		zap.String("user_id", payload.UserID.String()),
	)
}

func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// maskEmail returns "e***@domain.com" for logging.
func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || len(parts[0]) == 0 {
		return "***"
	}
	return string(parts[0][0]) + "***@" + parts[1]
}
