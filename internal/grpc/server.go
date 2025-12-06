package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"metachat/user-service/internal/models"
	"metachat/user-service/internal/service"

	pb "github.com/kegazani/metachat-proto/user"

	"github.com/kegazani/metachat-event-sourcing/aggregates"
	"github.com/kegazani/metachat-event-sourcing/events"
)

// UserServer implements the gRPC UserService interface
type UserServer struct {
	pb.UnimplementedUserServiceServer
	userService service.UserService
	logger      *logrus.Logger
}

// NewUserServer creates a new user gRPC server
func NewUserServer(userService service.UserService, logger *logrus.Logger) *UserServer {
	return &UserServer{
		userService: userService,
		logger:      logger,
	}
}

// GetUser retrieves a user by ID
func (s *UserServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	correlationID := GetCorrelationID(ctx)
	s.logger.WithFields(logrus.Fields{
		"correlation_id": correlationID,
		"user_id":        req.Id,
	}).Info("Getting user via gRPC")

	user, err := s.userService.GetUserByID(ctx, req.Id)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get user")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &pb.GetUserResponse{
		User: s.userToProto(user),
	}, nil
}

// UpdateUserProfile updates a user's profile
func (s *UserServer) UpdateUserProfile(ctx context.Context, req *pb.UpdateUserProfileRequest) (*pb.UpdateUserProfileResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Updating user profile via gRPC")

	user, err := s.userService.UpdateUserProfile(ctx, req.Id, req.FirstName, req.LastName, req.DateOfBirth, req.Avatar, req.Bio)
	if err != nil {
		s.logger.WithError(err).Error("Failed to update user profile")
		return nil, fmt.Errorf("failed to update user profile: %w", err)
	}

	return &pb.UpdateUserProfileResponse{
		User: s.userToProto(user),
	}, nil
}

// AssignArchetype assigns an archetype to a user
func (s *UserServer) AssignArchetype(ctx context.Context, req *pb.AssignArchetypeRequest) (*pb.AssignArchetypeResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Assigning archetype via gRPC")

	user, err := s.userService.AssignArchetype(ctx, req.Id, req.ArchetypeId, req.ArchetypeName, req.Confidence, req.Description)
	if err != nil {
		s.logger.WithError(err).Error("Failed to assign archetype")
		return nil, fmt.Errorf("failed to assign archetype: %w", err)
	}

	return &pb.AssignArchetypeResponse{
		User: s.userToProto(user),
	}, nil
}

// UpdateArchetype updates a user's archetype
func (s *UserServer) UpdateArchetype(ctx context.Context, req *pb.UpdateArchetypeRequest) (*pb.UpdateArchetypeResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Updating archetype via gRPC")

	user, err := s.userService.UpdateArchetype(ctx, req.Id, req.ArchetypeId, req.ArchetypeName, req.Confidence, req.Description)
	if err != nil {
		s.logger.WithError(err).Error("Failed to update archetype")
		return nil, fmt.Errorf("failed to update archetype: %w", err)
	}

	return &pb.UpdateArchetypeResponse{
		User: s.userToProto(user),
	}, nil
}

// UpdateModalities updates a user's modalities
func (s *UserServer) UpdateModalities(ctx context.Context, req *pb.UpdateModalitiesRequest) (*pb.UpdateModalitiesResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Updating modalities via gRPC")

	modalities := make([]events.UserModality, len(req.Modalities))
	for i, modality := range req.Modalities {
		modalities[i] = events.UserModality{
			ID:     modality.Id,
			Name:   modality.Name,
			Weight: modality.Score,
		}
	}

	user, err := s.userService.UpdateModalities(ctx, req.Id, modalities)
	if err != nil {
		s.logger.WithError(err).Error("Failed to update modalities")
		return nil, fmt.Errorf("failed to update modalities: %w", err)
	}

	return &pb.UpdateModalitiesResponse{
		User: s.userToProto(user),
	}, nil
}

// GetUserReadModel retrieves a user read model
func (s *UserServer) GetUserReadModel(ctx context.Context, req *pb.GetUserReadModelRequest) (*pb.GetUserReadModelResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Getting user read model via gRPC")

	user, err := s.userService.GetUserReadModelByID(ctx, req.Id)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get user read model")
		return nil, fmt.Errorf("failed to get user read model: %w", err)
	}

	return &pb.GetUserReadModelResponse{
		User: s.userReadModelToProto(user),
	}, nil
}

// GetUserArchetypeReadModel retrieves a user archetype read model
func (s *UserServer) GetUserArchetypeReadModel(ctx context.Context, req *pb.GetUserArchetypeReadModelRequest) (*pb.GetUserArchetypeReadModelResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Getting user archetype read model via gRPC")

	// For now, we'll get the user and extract the archetype from the read model
	user, err := s.userService.GetUserReadModelByID(ctx, req.Id)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get user archetype read model")
		return nil, fmt.Errorf("failed to get user archetype read model: %w", err)
	}

	return &pb.GetUserArchetypeReadModelResponse{
		UserArchetype: s.userArchetypeReadModelToProto(user),
	}, nil
}

// ListUsers lists users with pagination
func (s *UserServer) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	s.logger.WithFields(logrus.Fields{
		"page":   req.Page,
		"limit":  req.Limit,
		"filter": req.Filter,
	}).Info("Listing users via gRPC")

	// For now, return an empty list as ListUsers is not implemented in the service
	users := []*aggregates.UserAggregate{}
	total := 0

	protoUsers := make([]*pb.User, len(users))
	for i, user := range users {
		protoUsers[i] = s.userToProto(user)
	}

	return &pb.ListUsersResponse{
		Users: protoUsers,
		Total: int32(total),
		Page:  req.Page,
		Limit: req.Limit,
	}, nil
}

func (s *UserServer) GetUserProfileProgress(ctx context.Context, req *pb.GetUserProfileProgressRequest) (*pb.GetUserProfileProgressResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Getting user profile progress via gRPC")

	progress, err := s.userService.GetUserProfileProgress(ctx, req.Id)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get user profile progress")
		return nil, fmt.Errorf("failed to get user profile progress: %w", err)
	}

	return &pb.GetUserProfileProgressResponse{
		Progress: &pb.ProfileProgress{
			TokensAnalyzed:          progress.TokensAnalyzed,
			TokensRequiredForFirst:  progress.TokensRequiredForFirst,
			TokensRequiredForRecalc: progress.TokensRequiredForRecalc,
			DaysSinceLastCalc:       progress.DaysSinceLastCalc,
			DaysUntilRecalc:         progress.DaysUntilRecalc,
			IsFirstCalculation:      progress.IsFirstCalculation,
			ProgressPercentage:      progress.ProgressPercentage,
		},
	}, nil
}

func (s *UserServer) GetUserStatistics(ctx context.Context, req *pb.GetUserStatisticsRequest) (*pb.GetUserStatisticsResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Getting user statistics via gRPC")

	statistics, err := s.userService.GetUserStatistics(ctx, req.Id)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get user statistics")
		return nil, fmt.Errorf("failed to get user statistics: %w", err)
	}

	return &pb.GetUserStatisticsResponse{
		Statistics: &pb.UserStatistics{
			TotalDiaryEntries:     statistics.TotalDiaryEntries,
			TotalMoodAnalyses:     statistics.TotalMoodAnalyses,
			TotalTokens:           statistics.TotalTokens,
			DominantEmotion:       statistics.DominantEmotion,
			TopTopics:             statistics.TopTopics,
			ProfileCreatedAt:      timestamppb.New(statistics.ProfileCreatedAt),
			LastPersonalityUpdate: timestamppb.New(statistics.LastPersonalityUpdate),
		},
	}, nil
}

// Login authenticates a user and returns a JWT token
func (s *UserServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	s.logger.WithField("email", req.Email).Info("Login request via gRPC")

	token, err := s.userService.Login(ctx, req.Email, req.Password)
	if err != nil {
		s.logger.WithError(err).Error("Failed to login")
		return nil, fmt.Errorf("failed to login: %w", err)
	}

	readModel, err := s.userService.GetUserReadModelByEmail(ctx, req.Email)
	if err != nil {
		s.logger.WithError(err).Error("Failed to get user read model after login")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &pb.LoginResponse{
		Token: token,
		Id:    readModel.ID,
	}, nil
}

// Register creates a new user with password and returns a JWT token
func (s *UserServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	s.logger.WithFields(logrus.Fields{
		"email":    req.Email,
		"username": req.Username,
	}).Info("Register request via gRPC")

	if req.Username == "" {
		s.logger.Error("Username is required")
		return nil, fmt.Errorf("username is required")
	}
	if req.Email == "" {
		s.logger.Error("Email is required")
		return nil, fmt.Errorf("email is required")
	}
	if req.Password == "" {
		s.logger.Error("Password is required")
		return nil, fmt.Errorf("password is required")
	}

	token, err := s.userService.Register(ctx, req.Username, req.Email, req.Password, req.FirstName, req.LastName)
	if err != nil {
		s.logger.WithFields(logrus.Fields{
			"error":    err.Error(),
			"email":    req.Email,
			"username": req.Username,
		}).Error("Failed to register user")

		if errors.Is(err, service.ErrUsernameAlreadyExists) {
			return nil, status.Error(codes.AlreadyExists, "username already exists")
		}
		if errors.Is(err, service.ErrEmailAlreadyExists) {
			return nil, status.Error(codes.AlreadyExists, "email already exists")
		}

		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to register user: %v", err))
	}

	user, err := s.userService.GetUserByEmail(ctx, req.Email)
	if err != nil {
		s.logger.WithError(err).WithField("email", req.Email).Error("Failed to get user after registration")
		return nil, fmt.Errorf("failed to get user after registration: %w", err)
	}

	return &pb.RegisterResponse{
		Token: token,
		Id:    user.GetID(),
	}, nil
}

// userToProto converts a user aggregate to a protobuf user message
func (s *UserServer) userToProto(user *aggregates.UserAggregate) *pb.User {
	return &pb.User{
		Id:          user.GetID(),
		Username:    user.GetUsername(),
		Email:       user.GetEmail(),
		FirstName:   user.GetFirstName(),
		LastName:    user.GetLastName(),
		DateOfBirth: user.GetDateOfBirth(),
		Avatar:      user.GetAvatar(),
		Bio:         user.GetBio(),
		CreatedAt:   timestamppb.New(user.GetCreatedAt()),
		UpdatedAt:   timestamppb.New(user.GetUpdatedAt()),
	}
}

// userReadModelToProto converts a user read model to a protobuf user read model message
func (s *UserServer) userReadModelToProto(user *models.UserReadModel) *pb.UserReadModel {
	return &pb.UserReadModel{
		Id:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		FullName:  user.FirstName + " " + user.LastName,
		Avatar:    user.Avatar,
		Bio:       user.Bio,
		CreatedAt: timestamppb.New(user.CreatedAt),
		UpdatedAt: timestamppb.New(user.UpdatedAt),
	}
}

// userArchetypeReadModelToProto converts a user archetype read model to a protobuf user archetype read model message
func (s *UserServer) userArchetypeReadModelToProto(user *models.UserReadModel) *pb.UserArchetypeReadModel {
	return &pb.UserArchetypeReadModel{
		Id:            user.ID,
		ArchetypeId:   user.ArchetypeID,
		ArchetypeName: user.ArchetypeName,
		Description:   user.ArchetypeDescription,
		Confidence:    user.ArchetypeScore,
		AssignedAt:    timestamppb.Now(), // Using now as we don't have assigned_at in the read model
		UpdatedAt:     timestamppb.New(user.UpdatedAt),
	}
}

// StartGRPCServer starts the gRPC server
func StartGRPCServer(userService service.UserService, logger *logrus.Logger, port string) error {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	s := grpc.NewServer(
		grpc.UnaryInterceptor(CorrelationIDInterceptor(logger)),
	)
	pb.RegisterUserServiceServer(s, NewUserServer(userService, logger))

	// Enable reflection for development
	reflection.Register(s)

	logger.WithField("port", port).Info("Starting gRPC server")

	return s.Serve(lis)
}
