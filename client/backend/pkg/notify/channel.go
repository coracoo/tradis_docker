// Package notify contains external notification delivery primitives. It does
// not depend on HTTP handlers or task implementations so business operations
// can record a notification without waiting for a remote endpoint.
package notify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"dockerpanel/backend/pkg/database"
	"dockerpanel/backend/pkg/secrets"
)

const (
	ChannelTypeWebhook  = "webhook"
	ChannelTypeWeCom    = "wecom"
	ChannelTypeNtfy     = "ntfy"
	ChannelTypeGotify   = "gotify"
	ChannelTypeBark     = "bark"
	ChannelTypePushPlus = "pushplus"

	DeliveryPending   = "pending"
	DeliverySucceeded = "succeeded"
	DeliveryFailed    = "failed"
)

var ErrDeliveryFailed = errors.New("外部通知投递失败")

// Channel holds a delivery-ready channel. Secrets are intentionally excluded
// from JSON responses and only populated for the background dispatcher.
type Channel struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	Enabled    bool              `json:"enabled"`
	Categories []string          `json:"categories"`
	Config     map[string]string `json:"config"`
	Secrets    map[string]string `json:"-"`
	SecretSet  bool              `json:"secretSet"`
	CreatedAt  string            `json:"createdAt"`
	UpdatedAt  string            `json:"updatedAt"`
}

func (c Channel) SupportsCategory(category string) bool {
	if len(c.Categories) == 0 {
		return true
	}
	for _, value := range c.Categories {
		if strings.TrimSpace(value) == strings.TrimSpace(category) {
			return true
		}
	}
	return false
}

type Event struct {
	ID             string    `json:"id"`
	EnvironmentID  string    `json:"environmentId"`
	NotificationID int64     `json:"notificationId,omitempty"`
	Category       string    `json:"category"`
	Type           string    `json:"type"`
	Level          string    `json:"level"`
	Message        string    `json:"message"`
	Occurred       time.Time `json:"occurredAt"`
}

func (e Event) DedupeKey() string {
	occurred := e.Occurred
	if occurred.IsZero() {
		occurred = time.Now()
	}
	payload := strings.Join([]string{
		strings.TrimSpace(e.ID),
		strings.TrimSpace(e.Category),
		strings.TrimSpace(e.Type),
		strings.TrimSpace(e.Message),
		occurred.UTC().Truncate(5 * time.Minute).Format(time.RFC3339),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

type Delivery struct {
	ID             int64  `json:"id"`
	EnvironmentID  string `json:"environmentId"`
	ChannelID      string `json:"channelId"`
	NotificationID int64  `json:"notificationId,omitempty"`
	DedupeKey      string `json:"-"`
	EventCategory  string `json:"eventCategory"`
	EventType      string `json:"eventType"`
	Status         string `json:"status"`
	Attempts       int    `json:"attempts"`
	ResponseCode   int    `json:"responseCode,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type Store interface {
	ListEnabledChannels(context.Context, string) ([]Channel, error)
	CreateDelivery(context.Context, Delivery) (Delivery, bool, error)
	UpdateDelivery(context.Context, Delivery) error
}

type Sender interface {
	Send(context.Context, Channel, Event) (int, error)
}

// Dispatcher retries a single endpoint a bounded number of times. A sender
// error is recorded on its delivery and deliberately does not become a task
// error for the caller.
type Dispatcher struct {
	Store       Store
	Sender      Sender
	MaxAttempts int
	RetryDelay  func(attempt int) time.Duration
}

func NewDispatcher(store Store, sender Sender) *Dispatcher {
	return &Dispatcher{
		Store:       store,
		Sender:      sender,
		MaxAttempts: 3,
		RetryDelay: func(attempt int) time.Duration {
			switch attempt {
			case 1:
				return time.Second
			default:
				return 5 * time.Second
			}
		},
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, event Event) error {
	if d == nil || d.Store == nil || d.Sender == nil {
		return nil
	}
	channels, err := d.Store.ListEnabledChannels(ctx, event.Category)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if err := d.DispatchChannel(ctx, channel, event); err != nil && !errors.Is(err, ErrDeliveryFailed) {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) DispatchChannel(ctx context.Context, channel Channel, event Event) error {
	if d == nil || d.Store == nil || d.Sender == nil || !channel.Enabled {
		return nil
	}
	now := database.NowStamp()
	delivery, created, err := d.Store.CreateDelivery(ctx, Delivery{
		EnvironmentID:  event.EnvironmentID,
		ChannelID:      channel.ID,
		NotificationID: event.NotificationID,
		DedupeKey:      event.DedupeKey(),
		EventCategory:  event.Category,
		EventType:      event.Type,
		Status:         DeliveryPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil || !created {
		return err
	}

	attempts := d.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		code, sendErr := d.Sender.Send(ctx, channel, event)
		delivery.Attempts = attempt
		delivery.ResponseCode = code
		delivery.UpdatedAt = database.NowStamp()
		if sendErr == nil && code >= 200 && code < 300 {
			delivery.Status = DeliverySucceeded
			delivery.LastError = ""
			return d.Store.UpdateDelivery(ctx, delivery)
		}
		delivery.Status = DeliveryFailed
		if sendErr != nil {
			delivery.LastError = secrets.RedactString(sendErr.Error())
		} else {
			delivery.LastError = "远端返回非成功状态"
		}
		if err := d.Store.UpdateDelivery(ctx, delivery); err != nil {
			return err
		}
		if attempt < attempts {
			delay := time.Duration(0)
			if d.RetryDelay != nil {
				delay = d.RetryDelay(attempt)
			}
			if delay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		}
	}
	return fmt.Errorf("%w: %s", ErrDeliveryFailed, delivery.LastError)
}
