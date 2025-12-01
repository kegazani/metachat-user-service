package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/metachat/common/event-sourcing/aggregates"
	"github.com/metachat/common/event-sourcing/events"
	pb "github.com/metachat/proto/generated/user"
	"metachat/user-service/internal/models"
	"metachat/user-service/internal/service"
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

// CreateUser creates a new user
func (s *UserServer) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	s.logger.WithField("username", req.Username).Info("Creating user via gRPC")

	user, err := s.userService.CreateUser(ctx, req.Username, req.Email, req.FirstName, req.LastName, req.DateOfBirth)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create user")
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &pb.CreateUserResponse{
		User: s.userToProto(user),
	}, nil
}

// GetUser retrieves a user by ID
func (s *UserServer) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	s.logger.WithField("user_id", req.Id).Info("Getting user via gRPC")

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
			ID:      modality.Id,
			Name:    modality.Name,
			Type:    modality.Type,
			Enabled: modality.Enabled,
			Weight:  modality.Weight,
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

// userToProto converts a user aggregate to a protobuf user message
func (s *UserServer) userToProto(user *aggregates.UserAggregate) *pb.User {
	archetype := user.GetArchetype()
	var protoArchetype *pb.Archetype
	if archetype != nil {
		protoArchetype = &pb.Archetype{
			Id:          archetype.ID,
			Name:        archetype.Name,
			Description: archetype.Description,
			Confidence:  archetype.Score,
		}
	}

	modalities := user.GetModalities()
	protoModalities := make([]*pb.UserModality, len(modalities))
	for i, modality := range modalities {
		protoModalities[i] = &pb.UserModality{
			Id:      modality.ID,
			Name:    modality.Name,
			Type:    modality.Type,
			Enabled: modality.Enabled,
			Weight:  modality.Weight,
		}
	}

	// Get user data from the aggregate
	return &pb.User{
		Id:          user.GetID(),
		Username:    user.GetUsername(),
		Email:       user.GetEmail(),
		FirstName:   user.GetFirstName(),
		LastName:    user.GetLastName(),
		DateOfBirth: user.GetDateOfBirth(),
		Avatar:      user.GetAvatar(),
		Bio:         user.GetBio(),
		Archetype:   protoArchetype,
		Modalities:  protoModalities,
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

	s := grpc.NewServer()
	pb.RegisterUserServiceServer(s, NewUserServer(userService, logger))

	// Enable reflection for development
	reflection.Register(s)

	logger.WithField("port", port).Info("Starting gRPC server")

	return s.Serve(lis)
}
