package notify

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryStore struct {
	channels   []Channel
	deliveries map[string]Delivery
}

func (s *memoryStore) ListEnabledChannels(_ context.Context, category string) ([]Channel, error) {
	var result []Channel
	for _, channel := range s.channels {
		if channel.Enabled && channel.SupportsCategory(category) {
			result = append(result, channel)
		}
	}
	return result, nil
}

func (s *memoryStore) CreateDelivery(_ context.Context, delivery Delivery) (Delivery, bool, error) {
	if s.deliveries == nil {
		s.deliveries = map[string]Delivery{}
	}
	key := delivery.ChannelID + ":" + delivery.DedupeKey
	if existing, ok := s.deliveries[key]; ok {
		return existing, false, nil
	}
	delivery.ID = int64(len(s.deliveries) + 1)
	s.deliveries[key] = delivery
	return delivery, true, nil
}

func (s *memoryStore) UpdateDelivery(_ context.Context, delivery Delivery) error {
	key := delivery.ChannelID + ":" + delivery.DedupeKey
	s.deliveries[key] = delivery
	return nil
}

type scriptedSender struct {
	attempts int
	failFor  int
}

func (s *scriptedSender) Send(_ context.Context, _ Channel, _ Event) (int, error) {
	s.attempts++
	if s.attempts <= s.failFor {
		return 0, errors.New("temporary endpoint failure")
	}
	return 204, nil
}

func TestDispatcherRetriesThenRecordsSuccess(t *testing.T) {
	store := &memoryStore{channels: []Channel{{ID: "channel-1", Type: ChannelTypeWebhook, Enabled: true}}}
	sender := &scriptedSender{failFor: 2}
	dispatcher := NewDispatcher(store, sender)
	dispatcher.MaxAttempts = 3
	dispatcher.RetryDelay = func(int) time.Duration { return 0 }

	err := dispatcher.Dispatch(context.Background(), Event{
		ID:       "task-1",
		Category: "deploy_task",
		Type:     "deploy_completed",
		Message:  "应用部署完成",
		Occurred: time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	if sender.attempts != 3 {
		t.Fatalf("attempts = %d, want 3", sender.attempts)
	}
	for _, delivery := range store.deliveries {
		if delivery.Status != DeliverySucceeded || delivery.Attempts != 3 || delivery.ResponseCode != 204 {
			t.Fatalf("unexpected delivery: %#v", delivery)
		}
	}
}

func TestDispatcherDeduplicatesWithinWindow(t *testing.T) {
	store := &memoryStore{channels: []Channel{{ID: "channel-1", Type: ChannelTypeWebhook, Enabled: true}}}
	sender := &scriptedSender{}
	dispatcher := NewDispatcher(store, sender)
	dispatcher.RetryDelay = func(int) time.Duration { return 0 }

	event := Event{
		ID:       "task-1",
		Category: "deploy_task",
		Type:     "deploy_completed",
		Message:  "应用部署完成",
		Occurred: time.Date(2026, 7, 22, 10, 1, 0, 0, time.UTC),
	}
	if err := dispatcher.Dispatch(context.Background(), event); err != nil {
		t.Fatalf("first Dispatch() error = %v", err)
	}
	if err := dispatcher.Dispatch(context.Background(), event); err != nil {
		t.Fatalf("second Dispatch() error = %v", err)
	}
	if sender.attempts != 1 {
		t.Fatalf("sender attempts = %d, want 1", sender.attempts)
	}
	if len(store.deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(store.deliveries))
	}
}
