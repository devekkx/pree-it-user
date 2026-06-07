package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/devekkx/pree-it-user/db/sqlc"
	"github.com/devekkx/pree-it-user/internal/event"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

var (
	ErrProfileNotFound    = errors.New("profile not found")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrInvalidUsername    = errors.New("username must be 3-30 chars, alphanumeric/underscores only")
	ErrInvalidDisplayName = errors.New("display name must be 1-50 characters")
	ErrInvalidBio         = errors.New("bio must be 500 characters or less")
	ErrInvalidQuery       = errors.New("search query must be 1-50 characters")
)

var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,30}$`)

type ProfileService struct {
	queries   *sqlc.Queries
	publisher *event.Publisher
	logger    *zap.Logger
}

func NewProfileService(queries *sqlc.Queries, publisher *event.Publisher, logger *zap.Logger) *ProfileService {
	return &ProfileService{
		queries:   queries,
		publisher: publisher,
		logger:    logger,
	}
}

// CreateInitialProfile is called by the NATS consumer when a user registers.
// Implements event.ProfileCreator.
func (s *ProfileService) CreateInitialProfile(ctx context.Context, userID uuid.UUID, email string) error {
	ctx, span := otel.Tracer("user-service").Start(ctx, "service.CreateInitialProfile")
	defer span.End()

	// Derive initial display name and username from email.
	localPart := strings.SplitN(email, "@", 2)[0]

	displayName := localPart
	if utf8.RuneCountInString(displayName) > 50 {
		displayName = string([]rune(displayName)[:50])
	}

	username := sanitizeUsername(localPart)

	// Ensure username uniqueness by appending a suffix if needed.
	username, err := s.ensureUniqueUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("ensuring unique username: %w", err)
	}

	_, err = s.queries.CreateProfile(ctx, sqlc.CreateProfileParams{
		ID:          userID,
		DisplayName: displayName,
		Username:    username,
	})
	if err != nil {
		return fmt.Errorf("inserting profile: %w", err)
	}

	return nil
}

type GetProfileResult struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	Username    string    `json:"username"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	Bio         *string   `json:"bio,omitempty"`
	LastSeenAt  *string   `json:"last_seen_at,omitempty"`
	CreatedAt   string    `json:"created_at"`
}

func (s *ProfileService) GetProfileByID(ctx context.Context, userID uuid.UUID) (*GetProfileResult, error) {
	ctx, span := otel.Tracer("user-service").Start(ctx, "service.GetProfileByID")
	defer span.End()

	profile, err := s.queries.GetProfileByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProfileNotFound
		}
		return nil, fmt.Errorf("querying profile: %w", err)
	}

	return profileToResult(profile), nil
}

func (s *ProfileService) GetProfileByUsername(ctx context.Context, username string) (*GetProfileResult, error) {
	ctx, span := otel.Tracer("user-service").Start(ctx, "service.GetProfileByUsername")
	defer span.End()

	profile, err := s.queries.GetProfileByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProfileNotFound
		}
		return nil, fmt.Errorf("querying profile: %w", err)
	}

	return profileToResult(profile), nil
}

type UpdateProfileParams struct {
	DisplayName *string `json:"display_name"`
	Username    *string `json:"username"`
	AvatarURL   *string `json:"avatar_url"`
	Bio         *string `json:"bio"`
}

func (s *ProfileService) UpdateProfile(ctx context.Context, userID uuid.UUID, params UpdateProfileParams) (*GetProfileResult, error) {
	ctx, span := otel.Tracer("user-service").Start(ctx, "service.UpdateProfile")
	defer span.End()

	if err := s.validateUpdateParams(params); err != nil {
		return nil, err
	}

	// If username is changing, check uniqueness.
	if params.Username != nil {
		exists, err := s.queries.CheckUsernameExists(ctx, *params.Username)
		if err != nil {
			return nil, fmt.Errorf("checking username: %w", err)
		}
		if exists {
			// Might be the user's own username — verify.
			current, err := s.queries.GetProfileByID(ctx, userID)
			if err != nil {
				return nil, fmt.Errorf("fetching current profile: %w", err)
			}
			if current.Username != *params.Username {
				return nil, ErrUsernameTaken
			}
		}
	}

	dbParams := sqlc.UpdateProfileParams{ID: userID}

	if params.DisplayName != nil {
		dbParams.DisplayName = params.DisplayName
	}
	if params.Username != nil {
		dbParams.Username = params.Username
	}
	if params.AvatarURL != nil {
		dbParams.AvatarUrl = params.AvatarURL
	}
	if params.Bio != nil {
		dbParams.Bio = params.Bio
	}

	profile, err := s.queries.UpdateProfile(ctx, dbParams)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProfileNotFound
		}
		return nil, fmt.Errorf("updating profile: %w", err)
	}

	// Publish profile updated event.
	go func() {
		pubCtx := context.WithoutCancel(ctx)
		payload := event.ProfileUpdatedPayload{
			UserID:      profile.ID,
			DisplayName: profile.DisplayName,
			Username:    profile.Username,
		}
		if profile.AvatarUrl != nil {
			payload.AvatarURL = profile.AvatarUrl
		}
		if err := s.publisher.Publish(pubCtx, event.SubjectProfileUpdated, payload); err != nil {
			s.logger.Error("failed to publish profile.updated", zap.Error(err))
		}
	}()

	return profileToResult(profile), nil
}

type SearchParams struct {
	Query  string `form:"q" binding:"required"`
	Limit  int32  `form:"limit"`
	Offset int32  `form:"offset"`
}

func (s *ProfileService) SearchProfiles(ctx context.Context, params SearchParams) ([]GetProfileResult, error) {
	ctx, span := otel.Tracer("user-service").Start(ctx, "service.SearchProfiles")
	defer span.End()

	query := strings.TrimSpace(params.Query)
	if query == "" || utf8.RuneCountInString(query) > 50 {
		return nil, ErrInvalidQuery
	}

	if params.Limit <= 0 || params.Limit > 50 {
		params.Limit = 20
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	profiles, err := s.queries.SearchProfiles(ctx, sqlc.SearchProfilesParams{
		Query:  query,
		Limit:  params.Limit,
		Offset: params.Offset,
	})
	if err != nil {
		return nil, fmt.Errorf("searching profiles: %w", err)
	}

	results := make([]GetProfileResult, 0, len(profiles))
	for _, p := range profiles {
		results = append(results, *profileToResult(p))
	}
	return results, nil
}

func (s *ProfileService) DeactivateProfile(ctx context.Context, userID uuid.UUID) error {
	ctx, span := otel.Tracer("user-service").Start(ctx, "service.DeactivateProfile")
	defer span.End()

	if err := s.queries.DeactivateProfile(ctx, userID); err != nil {
		return fmt.Errorf("deactivating profile: %w", err)
	}

	go func() {
		pubCtx := context.WithoutCancel(ctx)
		payload := event.ProfileDeactivatedPayload{UserID: userID}
		if err := s.publisher.Publish(pubCtx, event.SubjectProfileDeactivated, payload); err != nil {
			s.logger.Error("failed to publish profile.deactivated", zap.Error(err))
		}
	}()

	return nil
}

func (s *ProfileService) UpdateLastSeen(ctx context.Context, userID uuid.UUID) error {
	return s.queries.UpdateLastSeen(ctx, userID)
}

func (s *ProfileService) GetProfilesByIDs(ctx context.Context, ids []uuid.UUID) ([]GetProfileResult, error) {
	ctx, span := otel.Tracer("user-service").Start(ctx, "service.GetProfilesByIDs")
	defer span.End()

	profiles, err := s.queries.GetProfilesByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("fetching profiles by IDs: %w", err)
	}

	results := make([]GetProfileResult, 0, len(profiles))
	for _, p := range profiles {
		results = append(results, *profileToResult(p))
	}
	return results, nil
}

// --- helpers ---

func (s *ProfileService) validateUpdateParams(params UpdateProfileParams) error {
	if params.DisplayName != nil {
		n := utf8.RuneCountInString(*params.DisplayName)
		if n < 1 || n > 50 {
			return ErrInvalidDisplayName
		}
	}
	if params.Username != nil && !usernameRegex.MatchString(*params.Username) {
		return ErrInvalidUsername
	}
	if params.Bio != nil && utf8.RuneCountInString(*params.Bio) > 500 {
		return ErrInvalidBio
	}
	return nil
}

func (s *ProfileService) ensureUniqueUsername(ctx context.Context, base string) (string, error) {
	candidate := base
	for attempt := 0; attempt < 10; attempt++ {
		exists, err := s.queries.CheckUsernameExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		// Append random suffix.
		suffix := uuid.New().String()[:6]
		candidate = base + "_" + suffix
		if len(candidate) > 30 {
			candidate = candidate[:30]
		}
	}
	return "", fmt.Errorf("could not generate unique username after 10 attempts")
}

func sanitizeUsername(input string) string {
	// Keep only alphanumeric and underscore.
	var b strings.Builder
	for _, r := range strings.ToLower(input) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if len(result) < 3 {
		result = result + "_user"
	}
	if len(result) > 30 {
		result = result[:30]
	}
	return result
}

func profileToResult(p sqlc.UserSchemaProfile) *GetProfileResult {
	result := &GetProfileResult{
		ID:          p.ID,
		DisplayName: p.DisplayName,
		Username:    p.Username,
		AvatarURL:   p.AvatarUrl,
		Bio:         p.Bio,
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if p.LastSeenAt.Valid {
		ts := p.LastSeenAt.Time.Format("2006-01-02T15:04:05Z")
		result.LastSeenAt = &ts
	}
	return result
}
