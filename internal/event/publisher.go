package event

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const (
	StreamName     = "USER_EVENTS"
	SubjectPrefix  = "user"
	StreamSubjects = "user.>"

	SubjectProfileUpdated     = "user.profile.updated"
	SubjectProfileDeactivated = "user.profile.deactivated"
)

// Envelope wraps every event published to NATS.
type Envelope struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

// ProfileUpdatedPayload is the data published when a profile changes.
type ProfileUpdatedPayload struct {
	UserID      uuid.UUID `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Username    string    `json:"username"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
}

// ProfileDeactivatedPayload is the data published when a profile is deactivated.
type ProfileDeactivatedPayload struct {
	UserID uuid.UUID `json:"user_id"`
}

type Publisher struct {
	js     jetstream.JetStream
	logger *zap.Logger
}

func NewPublisher(nc *nats.Conn, logger *zap.Logger) (*Publisher, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("creating jetstream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{StreamSubjects},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
		Replicas:  1,
	})
	if err != nil {
		return nil, fmt.Errorf("ensuring stream %s: %w", StreamName, err)
	}

	logger.Info("NATS JetStream publisher ready", zap.String("stream", StreamName))
	return &Publisher{js: js, logger: logger}, nil
}

func (p *Publisher) Publish(ctx context.Context, subject string, payload any) error {
	_, span := otel.Tracer("user-service").Start(ctx, "nats.publish."+subject)
	defer span.End()

	span.SetAttributes(attribute.String("nats.subject", subject))

	env := Envelope{
		ID:        uuid.New().String(),
		Type:      subject,
		Timestamp: time.Now().UTC(),
		Payload:   payload,
	}

	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	_, err = p.js.Publish(ctx, subject, data)
	if err != nil {
		p.logger.Error("failed to publish event",
			zap.String("subject", subject),
			zap.Error(err),
		)
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}

	p.logger.Debug("event published",
		zap.String("subject", subject),
		zap.String("event_id", env.ID),
	)
	return nil
}
