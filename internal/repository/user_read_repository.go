package repository

import (
	"context"
	"fmt"

	"metachat/user-service/internal/models"

	"github.com/kegazani/metachat-event-sourcing/events"

	"github.com/gocql/gocql"
)

// UserReadRepository defines the interface for user read model operations
type UserReadRepository interface {
	// SaveUser saves a user read model to Cassandra
	SaveUser(ctx context.Context, user *models.UserReadModel) error

	// SaveUserByUsername saves a user by username read model to Cassandra
	SaveUserByUsername(ctx context.Context, userByUsername *models.UserByUsernameReadModel) error

	// SaveUserByEmail saves a user by email read model to Cassandra
	SaveUserByEmail(ctx context.Context, userByEmail *models.UserByEmailReadModel) error

	// GetUserByID retrieves a user read model by ID
	GetUserByID(ctx context.Context, userID string) (*models.UserReadModel, error)

	// GetUserByUsername retrieves a user read model by username
	GetUserByUsername(ctx context.Context, username string) (*models.UserReadModel, error)

	// GetUserByEmail retrieves a user read model by email
	GetUserByEmail(ctx context.Context, email string) (*models.UserReadModel, error)

	// UpdateUser updates a user read model in Cassandra
	UpdateUser(ctx context.Context, user *models.UserReadModel) error

	// SaveUserWithIndexes saves a user read model along with its indexes
	SaveUserWithIndexes(ctx context.Context, user *models.UserReadModel) error

	// DeleteUser deletes a user read model from Cassandra
	DeleteUser(ctx context.Context, userID string) error

	// ProcessUserEvent processes a user event and updates the read models accordingly
	ProcessUserEvent(ctx context.Context, event *events.Event) error

	InitializeTables() error
}

// userReadRepository is the implementation of UserReadRepository
type userReadRepository struct {
	session *gocql.Session
}

// NewUserReadRepository creates a new user read repository
func NewUserReadRepository(session *gocql.Session) UserReadRepository {
	return &userReadRepository{
		session: session,
	}
}

// SaveUser saves a user read model to Cassandra
func (r *userReadRepository) SaveUser(ctx context.Context, user *models.UserReadModel) error {
	query := `INSERT INTO users_read_model (id, username, email, first_name, last_name, date_of_birth, 
		avatar, bio, archetype_id, archetype_name, archetype_score, archetype_description, 
		modalities, password_hash, created_at, updated_at, version) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	err := r.session.Query(query,
		user.ID, user.Username, user.Email, user.FirstName, user.LastName, user.DateOfBirth,
		user.Avatar, user.Bio, user.ArchetypeID, user.ArchetypeName, user.ArchetypeScore,
		user.ArchetypeDescription, user.Modalities, user.PasswordHash, user.CreatedAt, user.UpdatedAt, user.Version,
	).WithContext(ctx).Exec()
	
	if err != nil {
		return fmt.Errorf("cassandra error saving user read model: %w", err)
	}
	
	return nil
}

// SaveUserByUsername saves a user by username read model to Cassandra
func (r *userReadRepository) SaveUserByUsername(ctx context.Context, userByUsername *models.UserByUsernameReadModel) error {
	query := `INSERT INTO users_by_username_read_model (username, user_id, email) VALUES (?, ?, ?)`
	err := r.session.Query(query, userByUsername.Username, userByUsername.UserID, userByUsername.Email).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("cassandra error saving user by username index: %w", err)
	}
	return nil
}

// SaveUserByEmail saves a user by email read model to Cassandra
func (r *userReadRepository) SaveUserByEmail(ctx context.Context, userByEmail *models.UserByEmailReadModel) error {
	query := `INSERT INTO users_by_email_read_model (email, user_id, username) VALUES (?, ?, ?)`
	err := r.session.Query(query, userByEmail.Email, userByEmail.UserID, userByEmail.Username).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("cassandra error saving user by email index: %w", err)
	}
	return nil
}

// SaveUserWithIndexes saves a user read model along with its indexes
func (r *userReadRepository) SaveUserWithIndexes(ctx context.Context, user *models.UserReadModel) error {
	if err := r.SaveUser(ctx, user); err != nil {
		return fmt.Errorf("failed to save user read model: %w", err)
	}

	userByUsername := &models.UserByUsernameReadModel{
		Username: user.Username,
		UserID:   user.ID,
		Email:    user.Email,
	}
	if err := r.SaveUserByUsername(ctx, userByUsername); err != nil {
		return fmt.Errorf("failed to save user by username index: %w", err)
	}

	userByEmail := &models.UserByEmailReadModel{
		Email:    user.Email,
		UserID:   user.ID,
		Username: user.Username,
	}
	if err := r.SaveUserByEmail(ctx, userByEmail); err != nil {
		return fmt.Errorf("failed to save user by email index: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user read model by ID
func (r *userReadRepository) GetUserByID(ctx context.Context, userID string) (*models.UserReadModel, error) {
	query := `SELECT id, username, email, first_name, last_name, date_of_birth, 
		avatar, bio, archetype_id, archetype_name, archetype_score, archetype_description, 
		modalities, password_hash, created_at, updated_at, version 
		FROM users_read_model WHERE id = ?`

	var user models.UserReadModel
	err := r.session.Query(query, userID).Consistency(gocql.One).Scan(
		&user.ID, &user.Username, &user.Email, &user.FirstName, &user.LastName, &user.DateOfBirth,
		&user.Avatar, &user.Bio, &user.ArchetypeID, &user.ArchetypeName, &user.ArchetypeScore,
		&user.ArchetypeDescription, &user.Modalities, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt, &user.Version,
	)
	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("user not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetUserByUsername retrieves a user read model by username
func (r *userReadRepository) GetUserByUsername(ctx context.Context, username string) (*models.UserReadModel, error) {
	// First get the user ID from the username index
	var userID string
	err := r.session.Query(`SELECT user_id FROM users_by_username_read_model WHERE username = ?`, username).
		Consistency(gocql.One).Scan(&userID)
	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("username not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user ID by username: %w", err)
	}

	// Then get the full user model
	return r.GetUserByID(ctx, userID)
}

// GetUserByEmail retrieves a user read model by email
func (r *userReadRepository) GetUserByEmail(ctx context.Context, email string) (*models.UserReadModel, error) {
	// First get the user ID from the email index
	var userID string
	err := r.session.Query(`SELECT user_id FROM users_by_email_read_model WHERE email = ?`, email).
		Consistency(gocql.One).Scan(&userID)
	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, fmt.Errorf("email not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get user ID by email: %w", err)
	}

	// Then get the full user model
	return r.GetUserByID(ctx, userID)
}

// UpdateUser updates a user read model in Cassandra
func (r *userReadRepository) UpdateUser(ctx context.Context, user *models.UserReadModel) error {
	query := `UPDATE users_read_model SET username = ?, email = ?, first_name = ?, last_name = ?, 
		date_of_birth = ?, avatar = ?, bio = ?, archetype_id = ?, archetype_name = ?, 
		archetype_score = ?, archetype_description = ?, modalities = ?, password_hash = ?, updated_at = ?, version = ? 
		WHERE id = ?`

	return r.session.Query(query,
		user.Username, user.Email, user.FirstName, user.LastName, user.DateOfBirth,
		user.Avatar, user.Bio, user.ArchetypeID, user.ArchetypeName, user.ArchetypeScore,
		user.ArchetypeDescription, user.Modalities, user.PasswordHash, user.UpdatedAt, user.Version, user.ID,
	).Exec()
}

// DeleteUser deletes a user read model from Cassandra
func (r *userReadRepository) DeleteUser(ctx context.Context, userID string) error {
	// Get the user to retrieve username and email for index cleanup
	user, err := r.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	// Delete from main table
	if err := r.session.Query(`DELETE FROM users_read_model WHERE id = ?`, userID).Exec(); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Delete from username index
	if err := r.session.Query(`DELETE FROM users_by_username_read_model WHERE username = ?`, user.Username).Exec(); err != nil {
		return fmt.Errorf("failed to delete user by username: %w", err)
	}

	// Delete from email index
	if err := r.session.Query(`DELETE FROM users_by_email_read_model WHERE email = ?`, user.Email).Exec(); err != nil {
		return fmt.Errorf("failed to delete user by email: %w", err)
	}

	return nil
}

// ProcessUserEvent processes a user event and updates the read models accordingly
func (r *userReadRepository) ProcessUserEvent(ctx context.Context, event *events.Event) error {
	switch event.Type {
	case events.UserRegisteredEvent:
		return r.processUserRegisteredEvent(ctx, event)
	case events.UserProfileUpdatedEvent:
		return r.processUserProfileUpdatedEvent(ctx, event)
	case events.UserArchetypeAssignedEvent, events.UserArchetypeUpdatedEvent:
		return r.processUserArchetypeEvent(ctx, event)
	case events.UserModalitiesUpdatedEvent:
		return r.processUserModalitiesUpdatedEvent(ctx, event)
	default:
		return fmt.Errorf("unsupported event type: %s", event.Type)
	}
}

// processUserRegisteredEvent processes a UserRegistered event
func (r *userReadRepository) processUserRegisteredEvent(ctx context.Context, event *events.Event) error {
	// Create user read model
	user, err := models.NewUserReadModelFromEvent(event)
	if err != nil {
		return fmt.Errorf("failed to create user read model: %w", err)
	}

	// Create user by username read model
	userByUsername, err := models.NewUserByUsernameReadModelFromEvent(event)
	if err != nil {
		return fmt.Errorf("failed to create user by username read model: %w", err)
	}

	// Create user by email read model
	userByEmail, err := models.NewUserByEmailReadModelFromEvent(event)
	if err != nil {
		return fmt.Errorf("failed to create user by email read model: %w", err)
	}

	// Save all read models
	if err := r.SaveUser(ctx, user); err != nil {
		return fmt.Errorf("failed to save user read model: %w", err)
	}

	if err := r.SaveUserByUsername(ctx, userByUsername); err != nil {
		return fmt.Errorf("failed to save user by username read model: %w", err)
	}

	if err := r.SaveUserByEmail(ctx, userByEmail); err != nil {
		return fmt.Errorf("failed to save user by email read model: %w", err)
	}

	return nil
}

// processUserProfileUpdatedEvent processes a UserProfileUpdated event
func (r *userReadRepository) processUserProfileUpdatedEvent(ctx context.Context, event *events.Event) error {
	// Get existing user
	user, err := r.GetUserByID(ctx, event.AggregateID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Update user from event
	if err := user.UpdateFromProfileUpdateEvent(event); err != nil {
		return fmt.Errorf("failed to update user from event: %w", err)
	}

	// Save updated user
	if err := r.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// processUserArchetypeEvent processes a UserArchetypeAssigned or UserArchetypeUpdated event
func (r *userReadRepository) processUserArchetypeEvent(ctx context.Context, event *events.Event) error {
	// Get existing user
	user, err := r.GetUserByID(ctx, event.AggregateID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Update user from event
	if event.Type == events.UserArchetypeAssignedEvent {
		if err := user.UpdateFromArchetypeAssignedEvent(event); err != nil {
			return fmt.Errorf("failed to update user from archetype assigned event: %w", err)
		}
	} else {
		if err := user.UpdateFromArchetypeUpdatedEvent(event); err != nil {
			return fmt.Errorf("failed to update user from archetype updated event: %w", err)
		}
	}

	// Save updated user
	if err := r.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// processUserModalitiesUpdatedEvent processes a UserModalitiesUpdated event
func (r *userReadRepository) processUserModalitiesUpdatedEvent(ctx context.Context, event *events.Event) error {
	// Get existing user
	user, err := r.GetUserByID(ctx, event.AggregateID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Update user from event
	if err := user.UpdateFromModalitiesUpdatedEvent(event); err != nil {
		return fmt.Errorf("failed to update user from modalities updated event: %w", err)
	}

	// Save updated user
	if err := r.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// InitializeTables creates the necessary tables for user read models in Cassandra
func (r *userReadRepository) InitializeTables() error {
	// Create users_read_model table
	if err := r.session.Query(`CREATE TABLE IF NOT EXISTS users_read_model (
		id UUID PRIMARY KEY,
		username TEXT,
		email TEXT,
		first_name TEXT,
		last_name TEXT,
		date_of_birth TEXT,
		avatar TEXT,
		bio TEXT,
		archetype_id TEXT,
		archetype_name TEXT,
		archetype_score DOUBLE,
		archetype_description TEXT,
		modalities LIST<FROZEN<map<TEXT, TEXT>>>,
		password_hash TEXT,
		created_at TIMESTAMP,
		updated_at TIMESTAMP,
		version INT
	)`).Exec(); err != nil {
		return fmt.Errorf("failed to create users_read_model table: %w", err)
	}

	// Add password_hash column if it doesn't exist (for existing tables)
	r.session.Query(`ALTER TABLE users_read_model ADD password_hash TEXT`).Exec()

	// Create users_by_username_read_model table
	if err := r.session.Query(`CREATE TABLE IF NOT EXISTS users_by_username_read_model (
		username TEXT PRIMARY KEY,
		user_id UUID,
		email TEXT
	)`).Exec(); err != nil {
		return fmt.Errorf("failed to create users_by_username_read_model table: %w", err)
	}

	// Create users_by_email_read_model table
	if err := r.session.Query(`CREATE TABLE IF NOT EXISTS users_by_email_read_model (
		email TEXT PRIMARY KEY,
		user_id UUID,
		username TEXT
	)`).Exec(); err != nil {
		return fmt.Errorf("failed to create users_by_email_read_model table: %w", err)
	}

	return nil
}
