package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"go.uber.org/zap"
)

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	roles := auth.GetRoles(ctx)

	// Get user profile from database (use primary for read-after-write consistency)
	var email sql.NullString
	var emailVerified, phoneVerified bool
	var username, displayName, avatarURL, bio, country, phone sql.NullString
	var createdAt time.Time
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT COALESCE(email, ''), COALESCE(email_verified, FALSE), COALESCE(phone_verified, FALSE), username, display_name, avatar_url, bio, country, phone, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&email, &emailVerified, &phoneVerified, &username, &displayName, &avatarURL, &bio, &country, &phone, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.UserNotFound})
			return
		}
		a.log().Error("Failed to query user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	resp := UserResponse{
		UserID:        userID,
		Email:         email.String,
		Roles:         roles,
		EmailVerified: emailVerified,
		PhoneVerified: phoneVerified,
		CreatedAt:     createdAt,
	}

	// Set optional profile fields
	if username.Valid {
		resp.Username = &username.String
	}
	if displayName.Valid {
		resp.DisplayName = &displayName.String
	}
	if avatarURL.Valid {
		resp.AvatarURL = &avatarURL.String
	}
	if bio.Valid {
		resp.Bio = &bio.String
	}
	if country.Valid {
		resp.Country = &country.String
	}
	if phone.Valid {
		resp.Phone = &phone.String
	}

	writeJSON(w, http.StatusOK, resp)
}

// usernameRegex validates username format: alphanumeric + underscores, 3-30 chars.
var usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9_]{3,30}$`)

// countryCodeRegex validates ISO 3166-1 alpha-2 country codes.
var countryCodeRegex = regexp.MustCompile(`^[A-Z]{2}$`)

// phoneRegex validates E.164 phone number format.
var phoneRegex = regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)

// allowedProfileColumns is the safelist of columns allowed in dynamic profile UPDATE queries.
// This prevents accidental SQL injection if future modifications interpolate user input as column names.
var allowedProfileColumns = map[string]bool{
	"username": true, "display_name": true, "bio": true,
	"country": true, "phone": true,
}

// safeColumn validates that a column name is in the provided safelist, returning it unchanged.
// Panics if the column is not allowed — all callers use string literals, so a panic indicates
// a programming error, not a runtime condition.
func safeColumn(allowed map[string]bool, column string) string {
	if !allowed[column] {
		panic("disallowed SQL column: " + column)
	}
	return column
}

// handleUpdateProfile handles profile updates.
// PUT /api/user/me/profile
func (a *App) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Parse request body
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}

	// Validate inputs
	v := validation.New()

	// Validate username if provided
	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username != "" {
			if !usernameRegex.MatchString(username) {
				v.AddError("username", "invalid_format", "Username must be 3-30 alphanumeric characters or underscores")
			} else {
				// Check uniqueness
				var exists bool
				err := a.pool.Primary().QueryRowContext(ctx,
					`SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 AND id != $2)`,
					strings.ToLower(username), userID,
				).Scan(&exists)
				if err != nil {
					a.log().Error("Failed to check username uniqueness", zap.Error(err))
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
					return
				}
				if exists {
					v.AddError("username", "already_taken", "This username is already taken")
				}
			}
		}
	}

	// Validate display_name if provided
	if req.DisplayName != nil {
		displayName := strings.TrimSpace(*req.DisplayName)
		if displayName != "" && (len(displayName) < 2 || len(displayName) > 100) {
			v.AddError("display_name", "invalid_length", "Display name must be 2-100 characters")
		}
	}

	// Validate bio if provided
	if req.Bio != nil {
		bio := strings.TrimSpace(*req.Bio)
		if len(bio) > 500 {
			v.AddError("bio", "too_long", "Bio must not exceed 500 characters")
		}
	}

	// Validate country if provided
	if req.Country != nil {
		country := strings.TrimSpace(strings.ToUpper(*req.Country))
		if country != "" && !countryCodeRegex.MatchString(country) {
			v.AddError("country", "invalid_format", "Country must be a valid ISO 3166-1 alpha-2 code (e.g., US, GB)")
		}
	}

	// Validate phone if provided
	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		if phone != "" && !phoneRegex.MatchString(phone) {
			v.AddError("phone", "invalid_format", "Phone must be a valid E.164 format (e.g., +14155551234)")
		} else if phone != "" {
			// Check uniqueness
			var phoneTaken bool
			if err := a.pool.Replica().QueryRowContext(ctx,
				`SELECT EXISTS(SELECT 1 FROM users WHERE phone = $1 AND id != $2)`,
				phone, userID,
			).Scan(&phoneTaken); err != nil {
				a.log().Error("Failed to check phone uniqueness", zap.Error(err))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
				return
			} else if phoneTaken {
				v.AddError("phone", "already_taken", "This phone number is already in use")
			}
		}
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Build dynamic UPDATE query
	var updates []string
	var args []interface{}
	argIdx := 1

	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		col := safeColumn(allowedProfileColumns, "username")
		if username == "" {
			updates = append(updates, col+" = NULL")
		} else {
			updates = append(updates, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, strings.ToLower(username))
			argIdx++
		}
	}

	if req.DisplayName != nil {
		displayName := strings.TrimSpace(*req.DisplayName)
		col := safeColumn(allowedProfileColumns, "display_name")
		if displayName == "" {
			updates = append(updates, col+" = NULL")
		} else {
			updates = append(updates, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, displayName)
			argIdx++
		}
	}

	if req.Bio != nil {
		bio := strings.TrimSpace(*req.Bio)
		col := safeColumn(allowedProfileColumns, "bio")
		if bio == "" {
			updates = append(updates, col+" = NULL")
		} else {
			updates = append(updates, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, bio)
			argIdx++
		}
	}

	if req.Country != nil {
		country := strings.TrimSpace(strings.ToUpper(*req.Country))
		col := safeColumn(allowedProfileColumns, "country")
		if country == "" {
			updates = append(updates, col+" = NULL")
		} else {
			updates = append(updates, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, country)
			argIdx++
		}
	}

	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		col := safeColumn(allowedProfileColumns, "phone")
		if phone == "" {
			updates = append(updates, col+" = NULL")
		} else {
			updates = append(updates, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, phone)
			argIdx++
		}
	}

	// If no fields to update, return current profile
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, map[string]string{"message": msg.NoFieldsToUpdate})
		return
	}

	// Add user_id as the last argument
	args = append(args, userID)

	// Execute update
	// SAFETY: updates[] built via safeColumn(allowedProfileColumns, ...) — only whitelisted columns
	query := fmt.Sprintf("UPDATE users SET %s, updated_at = NOW() WHERE id = $%d",
		strings.Join(updates, ", "), argIdx)

	_, err := a.pool.Primary().ExecContext(ctx, query, args...)
	if err != nil {
		// Handle DB-level unique constraint violation as fallback for race conditions
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			if strings.Contains(err.Error(), "idx_users_phone") || strings.Contains(err.Error(), "phone") {
				writeJSON(w, http.StatusConflict, map[string]interface{}{
					"errors": []map[string]string{{"field": "phone", "code": "already_taken", "message": msg.PhoneAlreadyInUse}},
				})
			} else {
				writeJSON(w, http.StatusConflict, map[string]interface{}{
					"errors": []map[string]string{{"field": "unknown", "code": "already_taken", "message": msg.UniqueViolation}},
				})
			}
			return
		}
		a.log().Error("Failed to update profile", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Fetch updated profile
	var username, displayName, avatarURL, bio, country, phone sql.NullString
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT username, display_name, avatar_url, bio, country, phone FROM users WHERE id = $1`,
		userID,
	).Scan(&username, &displayName, &avatarURL, &bio, &country, &phone)
	if err != nil {
		a.log().Error("Failed to fetch updated profile", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	resp := UpdateProfileResponse{
		UserID: userID,
	}
	if username.Valid {
		resp.Username = &username.String
	}
	if displayName.Valid {
		resp.DisplayName = &displayName.String
	}
	if avatarURL.Valid {
		resp.AvatarURL = &avatarURL.String
	}
	if bio.Valid {
		resp.Bio = &bio.String
	}
	if country.Valid {
		resp.Country = &country.String
	}
	if phone.Valid {
		resp.Phone = &phone.String
	}

	a.log().Info("Profile updated",
		zap.String("user_id", userID))

	writeJSON(w, http.StatusOK, resp)
}

// handleUploadAvatar handles avatar image uploads.
// Uploads to S3/MinIO object storage for shared access across pods.
// POST /api/user/me/avatar
func (a *App) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Check that storage is configured
	if a.objectStorage == nil {
		a.log().Error("Avatar upload attempted but S3 storage not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.FileStorageUnavailable,
		})
		return
	}

	// Limit request body size to 2MB
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)

	// Parse multipart form
	if err := r.ParseMultipartForm(2 * 1024 * 1024); err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
				"error": msg.FileTooLarge,
			})
			return
		}
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidForm)
		return
	}

	// Get the file
	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, "avatar file is required")
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	allowedTypes := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/webp": "webp",
	}

	ext, ok := allowedTypes[contentType]
	if !ok {
		// Try to detect content type from file
		buff := make([]byte, 512)
		_, err := file.Read(buff)
		if err != nil && err != io.EOF {
			writeErrorJSON(w, r, http.StatusBadRequest, "failed to read file")
			return
		}
		file.Seek(0, 0) // Reset file pointer

		detectedType := http.DetectContentType(buff)
		ext, ok = allowedTypes[detectedType]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": msg.InvalidFileType,
			})
			return
		}
		contentType = detectedType
	}

	// Fetch old avatar URL for cleanup before uploading new one
	var oldAvatarURL sql.NullString
	_ = a.pool.Primary().QueryRowContext(ctx,
		`SELECT avatar_url FROM users WHERE id = $1`, userID,
	).Scan(&oldAvatarURL)

	// Generate unique object key and upload to S3/MinIO
	objectKey := fmt.Sprintf("avatars/%s_%d.%s", userID, time.Now().UnixNano(), ext)

	avatarURL, err := a.objectStorage.Upload(ctx, a.config.S3Bucket, objectKey, file, header.Size, contentType)
	if err != nil {
		a.log().Error("Failed to upload avatar to S3", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Update database with new avatar URL
	_, err = a.pool.Primary().ExecContext(ctx,
		`UPDATE users SET avatar_url = $1, updated_at = NOW() WHERE id = $2`,
		avatarURL, userID,
	)
	if err != nil {
		a.log().Error("Failed to update avatar URL", zap.Error(err))
		// Cleanup orphaned S3 object with retries in background
		bucket := a.config.S3Bucket
		infra.SafeGo(a.log(), "avatar-orphan-cleanup", func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cleanupCancel()
			for attempt := 1; attempt <= 3; attempt++ {
				if attempt > 1 {
					time.Sleep(time.Duration(attempt) * time.Second)
				}
				if delErr := a.objectStorage.Delete(cleanupCtx, bucket, objectKey); delErr != nil {
					a.log().Warn("Failed to clean up orphaned avatar",
						zap.Error(delErr), zap.String("key", objectKey), zap.Int("attempt", attempt))
					continue
				}
				return
			}
			a.log().Error("Gave up cleaning orphaned avatar after retries", zap.String("key", objectKey))
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Delete old avatar from S3 (best-effort, non-blocking)
	// Only delete custom uploads, not predefined avatars (/avatars/*.png served by nginx)
	if oldAvatarURL.Valid && strings.Contains(oldAvatarURL.String, "avatars/") && !strings.HasPrefix(oldAvatarURL.String, "/avatars/") {
		oldKey := extractAvatarObjectKey(oldAvatarURL.String, a.config.S3PublicURL)
		if oldKey != "" {
			infra.SafeGo(a.log(), "old-avatar-cleanup", func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cleanupCancel()
				if err := a.objectStorage.Delete(cleanupCtx, a.config.S3Bucket, oldKey); err != nil {
					a.log().Warn("Failed to delete old avatar",
						zap.Error(err), zap.String("old_key", oldKey))
				}
			})
		}
	}

	a.log().Info("Avatar uploaded",
		zap.String("user_id", userID),
		zap.String("avatar_url", avatarURL))

	writeJSON(w, http.StatusOK, AvatarUploadResponse{
		AvatarURL: avatarURL,
	})
}

// extractAvatarObjectKey extracts the S3 object key from a full avatar URL.
func extractAvatarObjectKey(avatarURL, publicURL string) string {
	if publicURL != "" {
		prefix := strings.TrimRight(publicURL, "/") + "/"
		if strings.HasPrefix(avatarURL, prefix) {
			return strings.TrimPrefix(avatarURL, prefix)
		}
	}
	// Fallback: look for "avatars/" in the URL path
	idx := strings.Index(avatarURL, "avatars/")
	if idx >= 0 {
		return avatarURL[idx:]
	}
	return ""
}

// handleSelectAvatar handles selecting a predefined avatar.
// POST /api/user/me/avatar/select
func (a *App) handleSelectAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Parse request body
	var req SelectAvatarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}

	// Validate avatar ID
	if req.AvatarID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.AvatarIDRequired,
		})
		return
	}

	// Look up avatar from DB (supports both UUID and slug for backward compatibility)
	var avatarPath, avatarSlug string
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT slug, image_path FROM predefined_avatars WHERE (id::text = $1 OR slug = $1) AND is_active = TRUE`,
		req.AvatarID,
	).Scan(&avatarSlug, &avatarPath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": msg.AvatarIDInvalid,
			})
			return
		}
		a.log().Error("Failed to query avatar", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Fetch old avatar URL for cleanup before updating
	var oldAvatarURL sql.NullString
	_ = a.pool.Primary().QueryRowContext(ctx,
		`SELECT avatar_url FROM users WHERE id = $1`, userID,
	).Scan(&oldAvatarURL)

	// Update database with new avatar URL
	_, err = a.pool.Primary().ExecContext(ctx,
		`UPDATE users SET avatar_url = $1, updated_at = NOW() WHERE id = $2`,
		avatarPath, userID,
	)
	if err != nil {
		a.log().Error("Failed to update avatar", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Delete old custom avatar from S3 (best-effort, non-blocking)
	// Only delete custom uploads, not predefined avatars (/avatars/*.png served by nginx)
	if a.objectStorage != nil && oldAvatarURL.Valid && strings.Contains(oldAvatarURL.String, "avatars/") && !strings.HasPrefix(oldAvatarURL.String, "/avatars/") {
		oldKey := extractAvatarObjectKey(oldAvatarURL.String, a.config.S3PublicURL)
		if oldKey != "" {
			infra.SafeGo(a.log(), "old-avatar-cleanup", func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cleanupCancel()
				if err := a.objectStorage.Delete(cleanupCtx, a.config.S3Bucket, oldKey); err != nil {
					a.log().Warn("Failed to delete old avatar",
						zap.Error(err), zap.String("old_key", oldKey))
				}
			})
		}
	}

	a.log().Info("Avatar selected",
		zap.String("user_id", userID),
		zap.String("avatar_id", avatarSlug),
		zap.String("avatar_url", avatarPath))

	writeJSON(w, http.StatusOK, SelectAvatarResponse{
		AvatarID:  avatarSlug,
		AvatarURL: avatarPath,
	})
}

// loadActiveAvatars loads active predefined avatars from the database.
func (a *App) loadActiveAvatars(ctx context.Context) ([]PredefinedAvatar, error) {
	rows, err := a.pool.Replica().QueryContext(ctx,
		`SELECT id, slug, display_name, category, bg_color, image_path, sort_order
		 FROM predefined_avatars
		 WHERE is_active = TRUE
		 ORDER BY sort_order ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var avatars []PredefinedAvatar
	for rows.Next() {
		var av PredefinedAvatar
		if err := rows.Scan(&av.ID, &av.Slug, &av.DisplayName, &av.Category, &av.BgColor, &av.Path, &av.SortOrder); err != nil {
			return nil, err
		}
		avatars = append(avatars, av)
	}
	return avatars, rows.Err()
}

// handleListAvatars returns the list of available predefined avatars.
// GET /api/user/avatars
func (a *App) handleListAvatars(w http.ResponseWriter, r *http.Request) {
	avatars, err := a.loadActiveAvatars(r.Context())
	if err != nil {
		a.log().Error("Failed to load avatars", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"avatars": avatars,
	})
}

// handleChangePassword handles authenticated password change requests.
// PUT /api/user/me/password
func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	currentSessionID := auth.GetSessionID(ctx)

	// Check rate limit (5 attempts per hour per user)
	if !a.passwordChangeRateLimiter.isAllowed(userID) {
		a.log().Warn("Password change rate limit exceeded",
			zap.String("user_id", userID))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": msg.TooManyPasswordChanges,
		})
		return
	}

	// Parse request body
	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, r, http.StatusBadRequest, msg.InvalidBody)
		return
	}

	// Validate passwords match
	if req.NewPassword != req.ConfirmPassword {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.PasswordsMismatch,
		})
		return
	}

	// Validate new password is different from current
	if req.CurrentPassword == req.NewPassword {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.PasswordSameAsOld,
		})
		return
	}

	// Validate input
	v := validation.New()
	v.Required("current_password", req.CurrentPassword)
	v.Password("new_password", req.NewPassword, validation.DefaultPasswordConstraints())

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Get current password hash from database
	var passwordHash string
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT password_hash FROM users WHERE id = $1`,
		userID,
	).Scan(&passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.UserNotFound})
			return
		}
		a.log().Error("Failed to query user password", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Verify current password
	if err := a.auth.VerifyPassword(req.CurrentPassword, passwordHash); err != nil {
		a.log().Warn("Password change failed - incorrect current password",
			zap.String("user_id", userID))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": msg.InvalidPassword})
		return
	}

	// Hash new password
	newPasswordHash, err := a.auth.HashPassword(req.NewPassword)
	if err != nil {
		a.log().Error("Failed to hash new password", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Update password and password_changed_at in database (P0-5: DB-backed session invalidation)
	_, err = a.pool.Primary().ExecContext(ctx,
		`UPDATE users SET password_hash = $1, password_changed_at = NOW(), updated_at = NOW() WHERE id = $2`,
		newPasswordHash, userID,
	)
	if err != nil {
		a.log().Error("Failed to update password", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Invalidate all other sessions (keep current session active)
	// Redis session deletion is an optimization; password_changed_at is the safety net
	if a.auth.Session != nil && currentSessionID != "" {
		if err := a.auth.Session.DeleteAllExcept(ctx, userID, currentSessionID); err != nil {
			// Log the error but don't fail the request - password was changed successfully
			a.log().Error("Failed to invalidate other sessions after password change",
				zap.String("user_id", userID),
				zap.Error(err))
		} else {
			a.log().Info("Other sessions invalidated after password change",
				zap.String("user_id", userID),
				zap.String("kept_session_id", currentSessionID))
		}
	}

	// Send password changed notification
	a.sendPasswordChangedNotification(ctx, userID, "panel_change", r)

	a.log().Info("Password changed successfully",
		zap.String("user_id", userID))

	writeJSON(w, http.StatusOK, map[string]string{"message": msg.PasswordChanged})
}

const contestsCacheTTL = 10 * time.Second

// handleListContests returns active/upcoming contests with symbols and rules.
// Supports filtering by: duration_type, asset_class, is_free, min_entry, max_entry
// Results are cached in memory for 10 seconds (only when no filters applied).
