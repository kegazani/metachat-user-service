package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/metachat/common/event-sourcing/aggregates"
	"github.com/metachat/common/event-sourcing/events"
	"metachat/user-service/internal/kafka"
	"metachat/user-service/internal/models"
	"metachat/user-service/internal/repository"

	"github.com/sirupsen/logrus"
)

// UserService defines the interface for user service operations
type UserService interface {
	// CreateUser creates a new user
	CreateUser(ctx context.Context, username, email, firstName, lastName, dateOfBirth string) (*aggregates.UserAggregate, error)

	// UpdateUserProfile updates a user's profile
	UpdateUserProfile(ctx context.Context, userID, firstName, lastName, dateOfBirth, avatar, bio string) (*aggregates.UserAggregate, error)

	// GetUserByID retrieves a user by ID
	GetUserByID(ctx context.Context, userID string) (*aggregates.UserAggregate, error)

	// GetUserByUsername retrieves a user by username
	GetUserByUsername(ctx context.Context, username string) (*aggregates.UserAggregate, error)

	// GetUserByEmail retrieves a user by email
	GetUserByEmail(ctx context.Context, email string) (*aggregates.UserAggregate, error)

	// AssignArchetype assigns an archetype to a user
	AssignArchetype(ctx context.Context, userID, archetypeID, archetypeName string, confidence float64, description string) (*aggregates.UserAggregate, error)

	// UpdateArchetype updates a user's archetype
	UpdateArchetype(ctx context.Context, userID, archetypeID, archetypeName string, confidence float64, description string) (*aggregates.UserAggregate, error)

	// UpdateModalities updates a user's modalities
	UpdateModalities(ctx context.Context, userID string, modalities []events.UserModality) (*aggregates.UserAggregate, error)

	// GetUserReadModelByID retrieves a user read model by ID
	GetUserReadModelByID(ctx context.Context, userID string) (*models.UserReadModel, error)

	// GetUserReadModelByUsername retrieves a user read model by username
	GetUserReadModelByUsername(ctx context.Context, username string) (*models.UserReadModel, error)

	// GetUserReadModelByEmail retrieves a user read model by email
	GetUserReadModelByEmail(ctx context.Context, email string) (*models.UserReadModel, error)
}

// userService is the implementation of UserService
type userService struct {
	userRepository     repository.UserRepository
	userReadRepository repository.UserReadRepository
	userEventProducer  kafka.UserEventProducer
}

// NewUserService creates a new user service
func NewUserService(userRepository repository.UserRepository, userReadRepository repository.UserReadRepository, userEventProducer kafka.UserEventProducer) UserService {
	return &userService{
		userRepository:     userRepository,
		userReadRepository: userReadRepository,
		userEventProducer:  userEventProducer,
	}
}

// CreateUser creates a new user
func (s *userService) CreateUser(ctx context.Context, username, email, firstName, lastName, dateOfBirth string) (*aggregates.UserAggregate, error) {
	// Check if user with username already exists
	if _, err := s.userRepository.GetUserByUsername(ctx, username); err == nil {
		return nil, ErrUsernameAlreadyExists
	}

	// Check if user with email already exists
	if _, err := s.userRepository.GetUserByEmail(ctx, email); err == nil {
		return nil, ErrEmailAlreadyExists
	}

	// Create new user aggregate
	user := aggregates.NewUserAggregate("")

	// Create user
	if err := user.CreateUser(username, email, firstName, lastName, dateOfBirth); err != nil {
		return nil, err
	}

	// Save user
	if err := s.userRepository.SaveUser(ctx, user); err != nil {
		return nil, err
	}

	// Publish event to Kafka
	if err := s.publishUserEvents(ctx, user); err != nil {
		logrus.WithError(err).Error("Failed to publish user events to Kafka")
	}

	return user, nil
}

// UpdateUserProfile updates a user's profile
func (s *userService) UpdateUserProfile(ctx context.Context, userID, firstName, lastName, dateOfBirth, avatar, bio string) (*aggregates.UserAggregate, error) {
	// Get user by ID
	user, err := s.userRepository.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update profile
	if err := user.UpdateProfile(firstName, lastName, dateOfBirth, avatar, bio); err != nil {
		return nil, err
	}

	// Save user
	if err := s.userRepository.SaveUser(ctx, user); err != nil {
		return nil, err
	}

	// Publish event to Kafka
	if err := s.publishUserEvents(ctx, user); err != nil {
		logrus.WithError(err).Error("Failed to publish user events to Kafka")
	}

	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *userService) GetUserByID(ctx context.Context, userID string) (*aggregates.UserAggregate, error) {
	return s.userRepository.GetUserByID(ctx, userID)
}

// GetUserByUsername retrieves a user by username
func (s *userService) GetUserByUsername(ctx context.Context, username string) (*aggregates.UserAggregate, error) {
	return s.userRepository.GetUserByUsername(ctx, username)
}

// GetUserByEmail retrieves a user by email
func (s *userService) GetUserByEmail(ctx context.Context, email string) (*aggregates.UserAggregate, error) {
	return s.userRepository.GetUserByEmail(ctx, email)
}

// AssignArchetype assigns an archetype to a user
func (s *userService) AssignArchetype(ctx context.Context, userID, archetypeID, archetypeName string, confidence float64, description string) (*aggregates.UserAggregate, error) {
	// Get user by ID
	user, err := s.userRepository.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Assign archetype
	if err := user.AssignArchetype(archetypeID, archetypeName, confidence, description); err != nil {
		return nil, err
	}

	// Save user
	if err := s.userRepository.SaveUser(ctx, user); err != nil {
		return nil, err
	}

	// Publish event to Kafka
	if err := s.publishUserEvents(ctx, user); err != nil {
		logrus.WithError(err).Error("Failed to publish user events to Kafka")
	}

	return user, nil
}

// UpdateArchetype updates a user's archetype
func (s *userService) UpdateArchetype(ctx context.Context, userID, archetypeID, archetypeName string, confidence float64, description string) (*aggregates.UserAggregate, error) {
	// Get user by ID
	user, err := s.userRepository.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update archetype
	if err := user.UpdateArchetype(archetypeID, archetypeName, confidence, description); err != nil {
		return nil, err
	}

	// Save user
	if err := s.userRepository.SaveUser(ctx, user); err != nil {
		return nil, err
	}

	// Publish event to Kafka
	if err := s.publishUserEvents(ctx, user); err != nil {
		logrus.WithError(err).Error("Failed to publish user events to Kafka")
	}

	return user, nil
}

// GetUserReadModelByID retrieves a user read model by ID
func (s *userService) GetUserReadModelByID(ctx context.Context, userID string) (*models.UserReadModel, error) {
	return s.userReadRepository.GetUserByID(ctx, userID)
}

// GetUserReadModelByUsername retrieves a user read model by username
func (s *userService) GetUserReadModelByUsername(ctx context.Context, username string) (*models.UserReadModel, error) {
	return s.userReadRepository.GetUserByUsername(ctx, username)
}

// GetUserReadModelByEmail retrieves a user read model by email
func (s *userService) GetUserReadModelByEmail(ctx context.Context, email string) (*models.UserReadModel, error) {
	return s.userReadRepository.GetUserByEmail(ctx, email)
}

// publishUserEvents publishes all uncommitted events for a user to Kafka
func (s *userService) publishUserEvents(ctx context.Context, user *aggregates.UserAggregate) error {
	events := user.GetUncommittedEvents()
	for _, event := range events {
		if err := s.userEventProducer.PublishUserEvent(ctx, event); err != nil {
			return fmt.Errorf("failed to publish user event: %w", err)
		}
	}

	// Mark events as committed
	user.ClearUncommittedEvents()
	return nil
}

// UpdateModalities updates a user's modalities
func (s *userService) UpdateModalities(ctx context.Context, userID string, modalities []events.UserModality) (*aggregates.UserAggregate, error) {
	// Get user by ID
	user, err := s.userRepository.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Update modalities
	if err := user.UpdateModalities(modalities); err != nil {
		return nil, err
	}

	// Save user
	if err := s.userRepository.SaveUser(ctx, user); err != nil {
		return nil, err
	}

	// Publish event to Kafka
	if err := s.publishUserEvents(ctx, user); err != nil {
		logrus.WithError(err).Error("Failed to publish user events to Kafka")
	}

	return user, nil
}

// Error definitions
var (
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrEmailAlreadyExists    = errors.New("email already exists")
)
