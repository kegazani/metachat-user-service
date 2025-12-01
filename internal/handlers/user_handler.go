package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/metachat/common/event-sourcing/aggregates"
	"github.com/metachat/common/event-sourcing/events"
	"github.com/metachat/common/event-sourcing/serializer"
	"github.com/metachat/common/event-sourcing/store"
	"metachat/user-service/internal/service"
)

// UserHandler handles HTTP requests for user operations
type UserHandler struct {
	userService          service.UserService
	userAggregateFactory func(string) aggregates.Aggregate
	eventStore           store.EventStore
	serializer           serializer.Serializer
	logger               *logrus.Logger
}

// NewUserHandler creates a new user handler
func NewUserHandler(
	userService service.UserService,
	userAggregateFactory func(string) aggregates.Aggregate,
	eventStore store.EventStore,
	serializer serializer.Serializer,
) *UserHandler {
	return &UserHandler{
		userService:          userService,
		userAggregateFactory: userAggregateFactory,
		eventStore:           eventStore,
		serializer:           serializer,
		logger:               logrus.New(),
	}
}

// RegisterRoutes registers the routes for the user handler
func (h *UserHandler) RegisterRoutes(router *mux.Router) {
	// User routes
	router.HandleFunc("/users", h.CreateUser).Methods("POST")
	router.HandleFunc("/users/{id}", h.GetUser).Methods("GET")
	router.HandleFunc("/users/{id}", h.UpdateUser).Methods("PUT")
	router.HandleFunc("/users/{id}/archetype", h.AssignArchetype).Methods("POST")
	router.HandleFunc("/users/{id}/archetype", h.UpdateArchetype).Methods("PUT")
	router.HandleFunc("/users/{id}/modalities", h.UpdateModalities).Methods("PUT")
}

// CreateUser handles the creation of a new user
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		FirstName   string `json:"first_name"`
		LastName    string `json:"last_name"`
		DateOfBirth string `json:"date_of_birth,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithError(err).Error("Failed to decode request body")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userService.CreateUser(ctx, req.Username, req.Email, req.FirstName, req.LastName, req.DateOfBirth)
	if err != nil {
		h.logger.WithError(err).Error("Failed to create user")
		switch err {
		case service.ErrUsernameAlreadyExists:
			http.Error(w, "Username already exists", http.StatusConflict)
		case service.ErrEmailAlreadyExists:
			http.Error(w, "Email already exists", http.StatusConflict)
		default:
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(h.userToResponse(user))
}

// GetUser handles retrieving a user by ID
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	userID := vars["id"]

	user, err := h.userService.GetUserByID(ctx, userID)
	if err != nil {
		h.logger.WithError(err).Error("Failed to get user")
		if err == store.ErrEventNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to get user", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.userToResponse(user))
}

// UpdateUser handles updating a user's profile
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	userID := vars["id"]

	var req struct {
		FirstName   string `json:"first_name,omitempty"`
		LastName    string `json:"last_name,omitempty"`
		DateOfBirth string `json:"date_of_birth,omitempty"`
		Avatar      string `json:"avatar,omitempty"`
		Bio         string `json:"bio,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithError(err).Error("Failed to decode request body")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userService.UpdateUserProfile(ctx, userID, req.FirstName, req.LastName, req.DateOfBirth, req.Avatar, req.Bio)
	if err != nil {
		h.logger.WithError(err).Error("Failed to update user")
		if err == store.ErrEventNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to update user", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.userToResponse(user))
}

// AssignArchetype handles assigning an archetype to a user
func (h *UserHandler) AssignArchetype(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	userID := vars["id"]

	var req struct {
		ArchetypeID   string  `json:"archetype_id"`
		ArchetypeName string  `json:"archetype_name"`
		Confidence    float64 `json:"confidence"`
		Description   string  `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithError(err).Error("Failed to decode request body")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userService.AssignArchetype(ctx, userID, req.ArchetypeID, req.ArchetypeName, req.Confidence, req.Description)
	if err != nil {
		h.logger.WithError(err).Error("Failed to assign archetype")
		if err == store.ErrEventNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to assign archetype", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.userToResponse(user))
}

// UpdateArchetype handles updating a user's archetype
func (h *UserHandler) UpdateArchetype(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	userID := vars["id"]

	var req struct {
		ArchetypeID   string  `json:"archetype_id"`
		ArchetypeName string  `json:"archetype_name"`
		Confidence    float64 `json:"confidence"`
		Description   string  `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithError(err).Error("Failed to decode request body")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userService.UpdateArchetype(ctx, userID, req.ArchetypeID, req.ArchetypeName, req.Confidence, req.Description)
	if err != nil {
		h.logger.WithError(err).Error("Failed to update archetype")
		if err == store.ErrEventNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to update archetype", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.userToResponse(user))
}

// UpdateModalities handles updating a user's modalities
func (h *UserHandler) UpdateModalities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	vars := mux.Vars(r)
	userID := vars["id"]

	var req struct {
		Modalities []events.UserModality `json:"modalities"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.WithError(err).Error("Failed to decode request body")
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := h.userService.UpdateModalities(ctx, userID, req.Modalities)
	if err != nil {
		h.logger.WithError(err).Error("Failed to update modalities")
		if err == store.ErrEventNotFound {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to update modalities", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.userToResponse(user))
}

// userToResponse converts a user aggregate to a response object
func (h *UserHandler) userToResponse(user *aggregates.UserAggregate) map[string]interface{} {
	archetype := user.GetArchetype()
	var archetypeResp interface{}
	if archetype != nil {
		archetypeResp = map[string]interface{}{
			"id":          archetype.ID,
			"name":        archetype.Name,
			"description": archetype.Description,
			"score":       archetype.Score,
		}
	}

	return map[string]interface{}{
		"id":         user.GetID(),
		"username":   user.GetUsername(),
		"email":      user.GetEmail(),
		"full_name":  user.GetFullName(),
		"archetype":  archetypeResp,
		"modalities": user.GetModalities(),
	}
}
