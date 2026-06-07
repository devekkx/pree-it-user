package handler

import (
	"errors"
	"net/http"

	"github.com/devekkx/pree-it-user/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Header set by the gateway after JWT validation.
const headerUserID = "X-User-ID"

type Handler struct {
	profiles *service.ProfileService
	logger   *zap.Logger
}

func New(profiles *service.ProfileService, logger *zap.Logger) *Handler {
	return &Handler{
		profiles: profiles,
		logger:   logger,
	}
}

// GetMyProfile returns the authenticated user's own profile.
// GET /api/v1/users/me
func (h *Handler) GetMyProfile(c *gin.Context) {
	userID, ok := h.extractUserID(c)
	if !ok {
		return
	}

	profile, err := h.profiles.GetProfileByID(c.Request.Context(), userID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": profile})
}

// GetProfileByID returns a profile by user ID.
// GET /api/v1/users/:id
func (h *Handler) GetProfileByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	profile, err := h.profiles.GetProfileByID(c.Request.Context(), id)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": profile})
}

// GetProfileByUsername returns a profile by username.
// GET /api/v1/users/username/:username
func (h *Handler) GetProfileByUsername(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	profile, err := h.profiles.GetProfileByUsername(c.Request.Context(), username)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": profile})
}

// UpdateMyProfile updates the authenticated user's profile.
// PATCH /api/v1/users/me
func (h *Handler) UpdateMyProfile(c *gin.Context) {
	userID, ok := h.extractUserID(c)
	if !ok {
		return
	}

	var params service.UpdateProfileParams
	if err := c.ShouldBindJSON(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	profile, err := h.profiles.UpdateProfile(c.Request.Context(), userID, params)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": profile})
}

// SearchProfiles searches profiles by display name or username.
// GET /api/v1/users/search?q=...&limit=20&offset=0
func (h *Handler) SearchProfiles(c *gin.Context) {
	var params service.SearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	results, err := h.profiles.SearchProfiles(c.Request.Context(), params)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": results, "count": len(results)})
}

// DeactivateMyProfile soft-deletes the authenticated user's profile.
// DELETE /api/v1/users/me
func (h *Handler) DeactivateMyProfile(c *gin.Context) {
	userID, ok := h.extractUserID(c)
	if !ok {
		return
	}

	if err := h.profiles.DeactivateProfile(c.Request.Context(), userID); err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "profile deactivated"})
}

// GetProfilesByIDs returns profiles for a list of user IDs (internal/batch use).
// POST /api/v1/users/batch
func (h *Handler) GetProfilesByIDs(c *gin.Context) {
	var req struct {
		IDs []uuid.UUID `json:"ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if len(req.IDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "maximum 100 IDs per request"})
		return
	}

	results, err := h.profiles.GetProfilesByIDs(c.Request.Context(), req.IDs)
	if err != nil {
		h.handleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": results, "count": len(results)})
}

// --- Health ---

func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "user-service"})
}

// --- Helpers ---

func (h *Handler) extractUserID(c *gin.Context) (uuid.UUID, bool) {
	raw := c.GetHeader(headerUserID)
	if raw == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user identity"})
		return uuid.Nil, false
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user identity"})
		return uuid.Nil, false
	}

	return id, true
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrProfileNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
	case errors.Is(err, service.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
	case errors.Is(err, service.ErrInvalidUsername):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidDisplayName):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidBio):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidQuery):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		h.logger.Error("unhandled error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}
}
