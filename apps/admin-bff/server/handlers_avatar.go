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
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// --- Types ---

// AdminAvatar represents a predefined avatar with full admin-visible fields.
type AdminAvatar struct {
	ID          string    `json:"id"`
	Slug        string    `json:"slug"`
	DisplayName string    `json:"display_name"`
	Category    string    `json:"category"`
	BgColor     string    `json:"bg_color"`
	ImagePath   string    `json:"image_path"`
	SortOrder   int       `json:"sort_order"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateAvatarRequest is unused as form fields are parsed from multipart.
// Kept for documentation.

// UpdateAvatarRequest represents fields that can be updated on a predefined avatar.
type UpdateAvatarRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Category    *string `json:"category,omitempty"`
	BgColor     *string `json:"bg_color,omitempty"`
	IsActive    *bool   `json:"is_active,omitempty"`
	SortOrder   *int    `json:"sort_order,omitempty"`
}

// ReorderAvatarsRequest represents a batch reorder operation.
type ReorderAvatarsRequest struct {
	Order []ReorderItem `json:"order"`
}

// ReorderItem represents a single item in a reorder operation.
type ReorderItem struct {
	ID        string `json:"id"`
	SortOrder int    `json:"sort_order"`
}

var avatarSlugRegex = regexp.MustCompile(`^[a-z0-9_-]{2,50}$`)

var validAvatarCategories = map[string]bool{"animal": true, "character": true, "special": true}

// --- Handlers ---

// handleListAdminAvatars returns ALL avatars (active + inactive) for admin panel.
// GET /api/admin/avatars
func (a *App) handleListAdminAvatars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := a.pool.Primary().QueryContext(ctx,
		`SELECT id, slug, display_name, category, bg_color, image_path, sort_order, is_active, created_at, updated_at
		 FROM predefined_avatars
		 ORDER BY sort_order ASC`)
	if err != nil {
		a.log().Error("Failed to list avatars", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer rows.Close()

	var avatars []AdminAvatar
	for rows.Next() {
		var av AdminAvatar
		if err := rows.Scan(&av.ID, &av.Slug, &av.DisplayName, &av.Category, &av.BgColor,
			&av.ImagePath, &av.SortOrder, &av.IsActive, &av.CreatedAt, &av.UpdatedAt); err != nil {
			a.log().Error("Failed to scan avatar", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		avatars = append(avatars, av)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate avatars", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"avatars": avatars,
		"total":   len(avatars),
	})
}

// handleCreateAdminAvatar creates a new predefined avatar with image upload.
// POST /api/admin/avatars (multipart form: slug, display_name, category, bg_color, image)
func (a *App) handleCreateAdminAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)

	if a.avatarStorage == nil {
		a.log().Error("Avatar upload attempted but S3 storage not configured")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.FileStorageNotConfigured,
		})
		return
	}

	// Limit to 2MB
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	if err := r.ParseMultipartForm(2 * 1024 * 1024); err != nil {
		if strings.Contains(err.Error(), "request body too large") || strings.Contains(err.Error(), "http: request body too large") {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": adminMsg.FileTooLarge})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidForm})
		return
	}

	slug := strings.TrimSpace(r.FormValue("slug"))
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	category := strings.TrimSpace(r.FormValue("category"))
	bgColor := strings.TrimSpace(r.FormValue("bg_color"))

	// Validate
	if slug == "" || displayName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugDisplayRequired})
		return
	}
	if !avatarSlugRegex.MatchString(slug) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugInvalid})
		return
	}
	if category == "" {
		category = "animal"
	}
	if !validAvatarCategories[category] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidCategory})
		return
	}
	if bgColor == "" {
		bgColor = "#2a2a3a"
	}

	// Check slug uniqueness
	var exists bool
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM predefined_avatars WHERE slug = $1)`, slug).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check slug", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if exists {
		writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.SlugExists})
		return
	}

	// Get image file
	file, header, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ImageRequired})
		return
	}
	defer file.Close()

	// Validate content type
	contentType := header.Header.Get("Content-Type")
	allowedTypes := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/webp": "webp",
	}
	ext, ok := allowedTypes[contentType]
	if !ok {
		buff := make([]byte, 512)
		_, readErr := file.Read(buff)
		if readErr != nil && readErr != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InternalError})
			return
		}
		if _, seekErr := file.Seek(0, 0); seekErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InternalError})
			return
		}
		detected := http.DetectContentType(buff)
		ext, ok = allowedTypes[detected]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidImageFormat})
			return
		}
		contentType = detected
	}

	// Upload to S3 under predefined-avatars/ prefix
	objectKey := fmt.Sprintf("predefined-avatars/%s.%s", slug, ext)
	imageURL, err := a.avatarStorage.Upload(ctx, a.config.S3AvatarBucket, objectKey, file, header.Size, contentType)
	if err != nil {
		a.log().Error("Failed to upload avatar image to S3", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Get next sort_order
	var maxOrder int
	_ = a.pool.Primary().QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), 0) FROM predefined_avatars`).Scan(&maxOrder)

	// Insert into DB
	var avatar AdminAvatar
	err = a.pool.Primary().QueryRowContext(ctx,
		`INSERT INTO predefined_avatars (slug, display_name, category, bg_color, image_path, sort_order)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, slug, display_name, category, bg_color, image_path, sort_order, is_active, created_at, updated_at`,
		slug, displayName, category, bgColor, imageURL, maxOrder+1,
	).Scan(&avatar.ID, &avatar.Slug, &avatar.DisplayName, &avatar.Category, &avatar.BgColor,
		&avatar.ImagePath, &avatar.SortOrder, &avatar.IsActive, &avatar.CreatedAt, &avatar.UpdatedAt)
	if err != nil {
		a.log().Error("Failed to insert avatar", zap.Error(err))
		// Cleanup orphaned S3 object
		infra.SafeGo(a.log(), "avatar-orphan-cleanup", func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = a.avatarStorage.Delete(cleanupCtx, a.config.S3AvatarBucket, objectKey)
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.logAuditEvent(ctx, adminID, "avatar.created", "avatar", avatar.ID, map[string]string{
		"slug": slug, "display_name": displayName,
	})

	a.log().Info("Admin avatar created", zap.String("slug", slug), zap.String("admin_id", adminID))
	writeJSON(w, http.StatusCreated, avatar)
}

// handleUpdateAdminAvatar updates avatar metadata (not image).
// PUT /api/admin/avatars/{id}
func (a *App) handleUpdateAdminAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)
	avatarID := chi.URLParam(r, "id")

	var req UpdateAvatarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Build dynamic update
	var updates []string
	var args []interface{}
	argIdx := 1

	if req.DisplayName != nil {
		updates = append(updates, fmt.Sprintf("display_name = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.DisplayName))
		argIdx++
	}
	if req.Category != nil {
		cat := strings.TrimSpace(*req.Category)
		if !validAvatarCategories[cat] {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidCategory})
			return
		}
		updates = append(updates, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, cat)
		argIdx++
	}
	if req.BgColor != nil {
		updates = append(updates, fmt.Sprintf("bg_color = $%d", argIdx))
		args = append(args, strings.TrimSpace(*req.BgColor))
		argIdx++
	}
	if req.IsActive != nil {
		updates = append(updates, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}
	if req.SortOrder != nil {
		updates = append(updates, fmt.Sprintf("sort_order = $%d", argIdx))
		args = append(args, *req.SortOrder)
		argIdx++
	}

	if len(updates) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}

	updates = append(updates, "updated_at = NOW()")
	args = append(args, avatarID)

	query := fmt.Sprintf("UPDATE predefined_avatars SET %s WHERE id = $%d RETURNING id, slug, display_name, category, bg_color, image_path, sort_order, is_active, created_at, updated_at",
		strings.Join(updates, ", "), argIdx)

	var avatar AdminAvatar
	err := a.pool.Primary().QueryRowContext(ctx, query, args...).Scan(
		&avatar.ID, &avatar.Slug, &avatar.DisplayName, &avatar.Category, &avatar.BgColor,
		&avatar.ImagePath, &avatar.SortOrder, &avatar.IsActive, &avatar.CreatedAt, &avatar.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.AvatarNotFound})
			return
		}
		a.log().Error("Failed to update avatar", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.logAuditEvent(ctx, adminID, "avatar.updated", "avatar", avatarID, req)
	writeJSON(w, http.StatusOK, avatar)
}

// handleReplaceAvatarImage replaces the image of an existing avatar.
// POST /api/admin/avatars/{id}/image (multipart form: image)
func (a *App) handleReplaceAvatarImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)
	avatarID := chi.URLParam(r, "id")

	if a.avatarStorage == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.FileStorageNotConfigured})
		return
	}

	// Get current avatar
	var slug, oldImagePath string
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT slug, image_path FROM predefined_avatars WHERE id = $1`, avatarID,
	).Scan(&slug, &oldImagePath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.AvatarNotFound})
			return
		}
		a.log().Error("Failed to query avatar", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	if err := r.ParseMultipartForm(2 * 1024 * 1024); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.FileTooLarge})
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ImageRequired})
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	allowedTypes := map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}
	ext, ok := allowedTypes[contentType]
	if !ok {
		buff := make([]byte, 512)
		_, readErr := file.Read(buff)
		if readErr != nil && readErr != io.EOF {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InternalError})
			return
		}
		if _, seekErr := file.Seek(0, 0); seekErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InternalError})
			return
		}
		detected := http.DetectContentType(buff)
		ext, ok = allowedTypes[detected]
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidImageFormat})
			return
		}
		contentType = detected
	}

	// Upload new image with timestamp to bust cache
	objectKey := fmt.Sprintf("predefined-avatars/%s_%d.%s", slug, time.Now().UnixNano(), ext)
	newURL, err := a.avatarStorage.Upload(ctx, a.config.S3AvatarBucket, objectKey, file, header.Size, contentType)
	if err != nil {
		a.log().Error("Failed to upload replacement image", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Update DB
	_, err = a.pool.Primary().ExecContext(ctx,
		`UPDATE predefined_avatars SET image_path = $1, updated_at = NOW() WHERE id = $2`,
		newURL, avatarID)
	if err != nil {
		a.log().Error("Failed to update image path", zap.Error(err))
		// Cleanup orphaned S3 object
		infra.SafeGo(a.log(), "avatar-replacement-orphan-cleanup", func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if delErr := a.avatarStorage.Delete(cleanupCtx, a.config.S3AvatarBucket, objectKey); delErr != nil {
				a.log().Warn("Failed to clean up orphaned replacement image",
					zap.Error(delErr), zap.String("key", objectKey))
			}
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Delete old S3 image (only if it's an S3 path, not nginx static)
	if !strings.HasPrefix(oldImagePath, "/avatars/") {
		infra.SafeGo(a.log(), "old-avatar-image-cleanup", func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			oldKey := extractPredefinedAvatarKey(oldImagePath, a.config.S3AvatarPublicURL)
			if oldKey != "" {
				_ = a.avatarStorage.Delete(cleanupCtx, a.config.S3AvatarBucket, oldKey)
			}
		})
	}

	a.logAuditEvent(ctx, adminID, "avatar.image_replaced", "avatar", avatarID, map[string]string{
		"slug": slug, "new_url": newURL,
	})
	writeJSON(w, http.StatusOK, map[string]string{"image_path": newURL})
}

// handleDeleteAdminAvatar soft-deletes an avatar (sets is_active=false).
// DELETE /api/admin/avatars/{id}
func (a *App) handleDeleteAdminAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)
	avatarID := chi.URLParam(r, "id")

	// Check if hard delete requested via query param
	hardDelete := r.URL.Query().Get("hard") == "true"

	if hardDelete {
		// Check no users are using this avatar
		var usageCount int
		err := a.pool.Primary().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM users WHERE avatar_url = (SELECT image_path FROM predefined_avatars WHERE id = $1)`,
			avatarID).Scan(&usageCount)
		if err != nil {
			a.log().Error("Failed to check avatar usage", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		if usageCount > 0 {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": adminMsg.AvatarInUse,
			})
			return
		}

		// Get image path for S3 cleanup
		var imagePath string
		err = a.pool.Primary().QueryRowContext(ctx,
			`SELECT image_path FROM predefined_avatars WHERE id = $1`, avatarID).Scan(&imagePath)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.AvatarNotFound})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}

		_, err = a.pool.Primary().ExecContext(ctx,
			`DELETE FROM predefined_avatars WHERE id = $1`, avatarID)
		if err != nil {
			a.log().Error("Failed to hard delete avatar", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}

		// Cleanup S3 (only non-static files)
		if a.avatarStorage != nil && !strings.HasPrefix(imagePath, "/avatars/") {
			infra.SafeGo(a.log(), "avatar-hard-delete-cleanup", func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				key := extractPredefinedAvatarKey(imagePath, a.config.S3AvatarPublicURL)
				if key != "" {
					_ = a.avatarStorage.Delete(cleanupCtx, a.config.S3AvatarBucket, key)
				}
			})
		}

		a.logAuditEvent(ctx, adminID, "avatar.hard_deleted", "avatar", avatarID, nil)
	} else {
		// Soft delete
		result, err := a.pool.Primary().ExecContext(ctx,
			`UPDATE predefined_avatars SET is_active = FALSE, updated_at = NOW() WHERE id = $1`, avatarID)
		if err != nil {
			a.log().Error("Failed to soft delete avatar", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.AvatarNotFound})
			return
		}
		a.logAuditEvent(ctx, adminID, "avatar.deactivated", "avatar", avatarID, nil)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.AvatarDeleted})
}

// handleReorderAvatars reorders avatars by updating sort_order.
// POST /api/admin/avatars/reorder
func (a *App) handleReorderAvatars(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	adminID := auth.GetUserID(ctx)

	var req ReorderAvatarsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	if len(req.Order) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	tx, err := a.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	for _, item := range req.Order {
		_, err := tx.ExecContext(ctx,
			`UPDATE predefined_avatars SET sort_order = $1, updated_at = NOW() WHERE id = $2`,
			item.SortOrder, item.ID)
		if err != nil {
			a.log().Error("Failed to update sort order", zap.Error(err), zap.String("id", item.ID))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit reorder", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.logAuditEvent(ctx, adminID, "avatar.reordered", "avatars", "", map[string]int{"count": len(req.Order)})
	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.AvatarsReordered})
}

// --- Helpers ---

// extractPredefinedAvatarKey extracts the S3 object key from an avatar image URL.
func extractPredefinedAvatarKey(imageURL, publicURL string) string {
	if publicURL != "" {
		prefix := strings.TrimRight(publicURL, "/") + "/"
		if strings.HasPrefix(imageURL, prefix) {
			return strings.TrimPrefix(imageURL, prefix)
		}
	}
	idx := strings.Index(imageURL, "predefined-avatars/")
	if idx >= 0 {
		return imageURL[idx:]
	}
	return ""
}
