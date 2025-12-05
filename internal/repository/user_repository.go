package repository

import (
	"context"
	"fmt"

	"github.com/kegazani/metachat-event-sourcing/aggregates"
	"github.com/kegazani/metachat-event-sourcing/events"
	"github.com/kegazani/metachat-event-sourcing/serializer"
	"github.com/kegazani/metachat-event-sourcing/store"
)

// UserRepository defines the interface for user repository operations
type UserRepository interface {
	// SaveUser saves a user aggregate to the event store
	SaveUser(ctx context.Context, user *aggregates.UserAggregate) error

	// GetUserByID retrieves a user aggregate by ID
	GetUserByID(ctx context.Context, userID string) (*aggregates.UserAggregate, error)

	// GetUserByUsername retrieves a user aggregate by username
	GetUserByUsername(ctx context.Context, username string) (*aggregates.UserAggregate, error)

	// GetUserByEmail retrieves a user aggregate by email
	GetUserByEmail(ctx context.Context, email string) (*aggregates.UserAggregate, error)
}

// userRepository is the implementation of UserRepository
type userRepository struct {
	eventStore store.EventStore
	serializer serializer.Serializer
}

// NewUserRepository creates a new user repository
func NewUserRepository(eventStore store.EventStore, serializer serializer.Serializer) UserRepository {
	return &userRepository{
		eventStore: eventStore,
		serializer: serializer,
	}
}

// SaveUser saves a user aggregate to the event store
func (r *userRepository) SaveUser(ctx context.Context, user *aggregates.UserAggregate) error {
	eventList := user.GetUncommittedEvents()
	if len(eventList) == 0 {
		return nil
	}

	if err := r.eventStore.SaveEvents(ctx, eventList); err != nil {
		if store.GetEventStoreErrorCode(err) == store.ErrCodeVersionConflict {
			return fmt.Errorf("version conflict while saving user events: %w", err)
		}
		if store.GetEventStoreErrorCode(err) == store.ErrCodeConnectionFailed {
			return fmt.Errorf("event store connection failed: %w", err)
		}
		return fmt.Errorf("failed to save events to event store: %w", err)
	}

	user.ClearUncommittedEvents()
	return nil
}

// GetUserByID retrieves a user aggregate by ID
func (r *userRepository) GetUserByID(ctx context.Context, userID string) (*aggregates.UserAggregate, error) {
	// Get events from event store
	events, err := r.eventStore.GetEventsByAggregateID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Create user aggregate
	user := aggregates.NewUserAggregate(userID)

	// Load events into aggregate
	if err := user.LoadFromHistory(events); err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByUsername retrieves a user aggregate by username
func (r *userRepository) GetUserByUsername(ctx context.Context, username string) (*aggregates.UserAggregate, error) {
	// Get all UserRegistered events
	eventList, err := r.eventStore.GetEventsByType(ctx, events.UserRegisteredEvent)
	if err != nil {
		return nil, err
	}

	// Find the user with the matching username
	for _, event := range eventList {
		var payload events.UserRegisteredPayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			continue
		}

		if payload.Username == username {
			// Create user aggregate
			user := aggregates.NewUserAggregate(event.AggregateID)

			// Load events into aggregate
			if err := user.LoadFromHistory([]*events.Event{event}); err != nil {
				return nil, err
			}

			return user, nil
		}
	}

	return nil, store.ErrEventNotFound
}

// GetUserByEmail retrieves a user aggregate by email
func (r *userRepository) GetUserByEmail(ctx context.Context, email string) (*aggregates.UserAggregate, error) {
	// Get all UserRegistered events
	eventList, err := r.eventStore.GetEventsByType(ctx, events.UserRegisteredEvent)
	if err != nil {
		return nil, err
	}

	// Find the user with the matching email
	for _, event := range eventList {
		var payload events.UserRegisteredPayload
		if err := event.UnmarshalPayload(&payload); err != nil {
			continue
		}

		if payload.Email == email {
			// Create user aggregate
			user := aggregates.NewUserAggregate(event.AggregateID)

			// Load events into aggregate
			if err := user.LoadFromHistory([]*events.Event{event}); err != nil {
				return nil, err
			}

			return user, nil
		}
	}

	return nil, store.ErrEventNotFound
}
