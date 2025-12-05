package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"metachat/user-service/internal/auth"
	"metachat/user-service/internal/kafka"
	"metachat/user-service/internal/models"
	"metachat/user-service/internal/repository"

	"github.com/google/uuid"
	"github.com/kegazani/metachat-event-sourcing/aggregates"
	"github.com/kegazani/metachat-event-sourcing/events"
	"github.com/kegazani/metachat-event-sourcing/store"

	"github.com/sirupsen/logrus"
)

const maxRetries = 3

type PersonalityServiceClient interface {
	GetProfileProgress(ctx context.Context, userID string) (*ProfileProgress, error)
}

type AnalyticsServiceClient interface {
	GetUserStatistics(ctx context.Context, userID string) (*UserStatistics, error)
}

// UserService defines the interface for user service operations
type UserService interface {
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

	// Register creates a new user with password
	Register(ctx context.Context, username, email, password, firstName, lastName string) (string, error)

	// Login authenticates a user and returns a JWT token
	Login(ctx context.Context, email, password string) (string, error)

	// OAuthLogin handles OAuth authentication
	OAuthLogin(ctx context.Context, provider, token string) (string, error)

	// GetUserProfileProgress retrieves profile progress for a user
	GetUserProfileProgress(ctx context.Context, userID string) (*ProfileProgress, error)

	// GetUserStatistics retrieves statistics for a user
	GetUserStatistics(ctx context.Context, userID string) (*UserStatistics, error)
}

type ProfileProgress struct {
	TokensAnalyzed          int32
	TokensRequiredForFirst  int32
	TokensRequiredForRecalc int32
	DaysSinceLastCalc       int32
	DaysUntilRecalc         int32
	IsFirstCalculation      bool
	ProgressPercentage      float64
}

type UserStatistics struct {
	TotalDiaryEntries      int32
	TotalMoodAnalyses      int32
	TotalTokens            int32
	DominantEmotion        string
	TopTopics              []string
	ProfileCreatedAt       time.Time
	LastPersonalityUpdate  time.Time
}

// userService is the implementation of UserService
type userService struct {
	userRepository     repository.UserRepository
	userReadRepository repository.UserReadRepository
	userEventProducer  kafka.UserEventProducer
	jwtManager         *auth.JWTManager
	googleOAuth        *auth.GoogleOAuthProvider
	appleOAuth         *auth.AppleOAuthProvider
	personalityClient  PersonalityServiceClient
	analyticsClient    AnalyticsServiceClient
}

// NewUserService creates a new user service
func NewUserService(userRepository repository.UserRepository, userReadRepository repository.UserReadRepository, userEventProducer kafka.UserEventProducer, jwtManager *auth.JWTManager, googleOAuth *auth.GoogleOAuthProvider, appleOAuth *auth.AppleOAuthProvider, personalityClient PersonalityServiceClient, analyticsClient AnalyticsServiceClient) UserService {
	return &userService{
		userRepository:     userRepository,
		userReadRepository: userReadRepository,
		userEventProducer:  userEventProducer,
		jwtManager:         jwtManager,
		googleOAuth:        googleOAuth,
		appleOAuth:         appleOAuth,
		personalityClient:  personalityClient,
		analyticsClient:    analyticsClient,
	}
}

// createUser creates a new user (internal method, not exposed via API)
func (s *userService) createUser(ctx context.Context, username, email, firstName, lastName, dateOfBirth string) (*aggregates.UserAggregate, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}

	existingUser, err := s.userRepository.GetUserByUsername(ctx, username)
	if err == nil && existingUser != nil {
		return nil, ErrUsernameAlreadyExists
	}
	if err != nil && err != store.ErrEventNotFound {
		return nil, fmt.Errorf("failed to check username existence: %w", err)
	}

	existingUser, err = s.userRepository.GetUserByEmail(ctx, email)
	if err == nil && existingUser != nil {
		return nil, ErrEmailAlreadyExists
	}
	if err != nil && err != store.ErrEventNotFound {
		return nil, fmt.Errorf("failed to check email existence: %w", err)
	}

	userID := uuid.New().String()

	user := aggregates.NewUserAggregate(userID)

	if err := user.CreateUser(username, email, firstName, lastName, dateOfBirth); err != nil {
		return nil, fmt.Errorf("failed to create user aggregate: %w", err)
	}

	if err := s.userRepository.SaveUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to save user to event store: %w", err)
	}

	if err := s.publishUserEvents(ctx, user); err != nil {
		logrus.WithError(err).Error("Failed to publish user events to Kafka")
	}

	return user, nil
}

// UpdateUserProfile updates a user's profile
func (s *userService) UpdateUserProfile(ctx context.Context, userID, firstName, lastName, dateOfBirth, avatar, bio string) (*aggregates.UserAggregate, error) {
	var user *aggregates.UserAggregate
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		user, err = s.userRepository.GetUserByID(ctx, userID)
		if err != nil {
			return nil, err
		}

		if err = user.UpdateProfile(firstName, lastName, dateOfBirth, avatar, bio); err != nil {
			return nil, err
		}

		if err = s.userRepository.SaveUser(ctx, user); err != nil {
			if isVersionConflict(err) {
				time.Sleep(time.Millisecond * time.Duration(10*(attempt+1)))
				continue
			}
			return nil, err
		}

		if err = s.publishUserEvents(ctx, user); err != nil {
			logrus.WithError(err).Error("Failed to publish user events to Kafka")
		}

		return user, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, err)
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
	var user *aggregates.UserAggregate
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		user, err = s.userRepository.GetUserByID(ctx, userID)
		if err != nil {
			return nil, err
		}

		if err = user.AssignArchetype(archetypeID, archetypeName, confidence, description); err != nil {
			return nil, err
		}

		if err = s.userRepository.SaveUser(ctx, user); err != nil {
			if isVersionConflict(err) {
				time.Sleep(time.Millisecond * time.Duration(10*(attempt+1)))
				continue
			}
			return nil, err
		}

		if err = s.publishUserEvents(ctx, user); err != nil {
			logrus.WithError(err).Error("Failed to publish user events to Kafka")
		}

		return user, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, err)
}

// UpdateArchetype updates a user's archetype
func (s *userService) UpdateArchetype(ctx context.Context, userID, archetypeID, archetypeName string, confidence float64, description string) (*aggregates.UserAggregate, error) {
	var user *aggregates.UserAggregate
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		user, err = s.userRepository.GetUserByID(ctx, userID)
		if err != nil {
			return nil, err
		}

		if err = user.UpdateArchetype(archetypeID, archetypeName, confidence, description); err != nil {
			return nil, err
		}

		if err = s.userRepository.SaveUser(ctx, user); err != nil {
			if isVersionConflict(err) {
				time.Sleep(time.Millisecond * time.Duration(10*(attempt+1)))
				continue
			}
			return nil, err
		}

		if err = s.publishUserEvents(ctx, user); err != nil {
			logrus.WithError(err).Error("Failed to publish user events to Kafka")
		}

		return user, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, err)
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
	var user *aggregates.UserAggregate
	var err error

	for attempt := 0; attempt < maxRetries; attempt++ {
		user, err = s.userRepository.GetUserByID(ctx, userID)
		if err != nil {
			return nil, err
		}

		if err = user.UpdateModalities(modalities); err != nil {
			return nil, err
		}

		if err = s.userRepository.SaveUser(ctx, user); err != nil {
			if isVersionConflict(err) {
				time.Sleep(time.Millisecond * time.Duration(10*(attempt+1)))
				continue
			}
			return nil, err
		}

		if err = s.publishUserEvents(ctx, user); err != nil {
			logrus.WithError(err).Error("Failed to publish user events to Kafka")
		}

		return user, nil
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, err)
}

// Register creates a new user with password
func (s *userService) Register(ctx context.Context, username, email, password, firstName, lastName string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password is required")
	}

	user, err := s.createUser(ctx, username, email, firstName, lastName, "")
	if err != nil {
		if err == ErrUsernameAlreadyExists || err == ErrEmailAlreadyExists {
			return "", err
		}
		return "", fmt.Errorf("failed to create user: %w", err)
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	readModel := &models.UserReadModel{
		ID:           user.GetID(),
		Username:     username,
		Email:        email,
		FirstName:    firstName,
		LastName:     lastName,
		DateOfBirth:  "",
		PasswordHash: passwordHash,
		CreatedAt:    user.GetCreatedAt(),
		UpdatedAt:    user.GetUpdatedAt(),
		Version:      user.GetVersion(),
	}

	if err := s.userReadRepository.SaveUserWithIndexes(ctx, readModel); err != nil {
		logrus.WithError(err).WithField("user_id", user.GetID()).Error("Failed to save user read model, user may be partially created")
		return "", fmt.Errorf("failed to save user read model: %w", err)
	}

	token, err := s.jwtManager.GenerateToken(user.GetID(), username, email)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

// Login authenticates a user and returns a JWT token
func (s *userService) Login(ctx context.Context, email, password string) (string, error) {
	readModel, err := s.GetUserReadModelByEmail(ctx, email)
	if err != nil {
		return "", auth.ErrInvalidCredentials
	}

	if !auth.CheckPasswordHash(password, readModel.PasswordHash) {
		return "", auth.ErrInvalidCredentials
	}

	user, err := s.GetUserByID(ctx, readModel.ID)
	if err != nil {
		return "", err
	}

	token, err := s.jwtManager.GenerateToken(user.GetID(), user.GetUsername(), user.GetEmail())
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

// OAuthLogin handles OAuth authentication
func (s *userService) OAuthLogin(ctx context.Context, provider, token string) (string, error) {
	var oauthProvider auth.OAuthProvider
	switch provider {
	case "google":
		oauthProvider = s.googleOAuth
	case "apple":
		oauthProvider = s.appleOAuth
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	userInfo, err := oauthProvider.GetUserInfo(ctx, token)
	if err != nil {
		return "", fmt.Errorf("failed to get user info: %w", err)
	}

	readModel, err := s.GetUserReadModelByEmail(ctx, userInfo.Email)
	if err != nil {
		user, createErr := s.createUser(ctx, userInfo.Email, userInfo.Email, userInfo.FirstName, userInfo.LastName, "")
		if createErr != nil {
			return "", createErr
		}
		readModel, _ = s.GetUserReadModelByID(ctx, user.GetID())
	}

	user, err := s.GetUserByID(ctx, readModel.ID)
	if err != nil {
		return "", err
	}

	jwtToken, err := s.jwtManager.GenerateToken(user.GetID(), user.GetUsername(), user.GetEmail())
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return jwtToken, nil
}

func isVersionConflict(err error) bool {
	return store.GetEventStoreErrorCode(err) == store.ErrCodeVersionConflict
}

func (s *userService) GetUserProfileProgress(ctx context.Context, userID string) (*ProfileProgress, error) {
	if s.personalityClient == nil {
		return &ProfileProgress{
			TokensAnalyzed:          0,
			TokensRequiredForFirst:  50,
			TokensRequiredForRecalc: 100,
			DaysSinceLastCalc:       0,
			DaysUntilRecalc:         0,
			IsFirstCalculation:      true,
			ProgressPercentage:      0.0,
		}, nil
	}

	return s.personalityClient.GetProfileProgress(ctx, userID)
}

func (s *userService) GetUserStatistics(ctx context.Context, userID string) (*UserStatistics, error) {
	if s.analyticsClient == nil {
		return &UserStatistics{
			TotalDiaryEntries:     0,
			TotalMoodAnalyses:      0,
			TotalTokens:            0,
			DominantEmotion:        "",
			TopTopics:              []string{},
			ProfileCreatedAt:       time.Time{},
			LastPersonalityUpdate:  time.Time{},
		}, nil
	}

	return s.analyticsClient.GetUserStatistics(ctx, userID)
}

var (
	ErrUsernameAlreadyExists = errors.New("username already exists")
	ErrEmailAlreadyExists    = errors.New("email already exists")
)
