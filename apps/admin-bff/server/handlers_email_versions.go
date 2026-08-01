package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// TemplateVersionListItem is the abbreviated form returned when listing versions.
type TemplateVersionListItem struct {
	ID          string          `json:"id"`
	Slug        string          `json:"slug"`
	VersionName string          `json:"version_name"`
	IsActive    bool            `json:"is_active"`
	FontConfig  json.RawMessage `json:"font_config"`
	CreatedBy   *string         `json:"created_by,omitempty"`
	UpdatedBy   *string         `json:"updated_by,omitempty"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// TemplateVersionDetail includes the full HTML and CSS content.
type TemplateVersionDetail struct {
	ID          string          `json:"id"`
	Slug        string          `json:"slug"`
	VersionName string          `json:"version_name"`
	HTMLBody    string          `json:"html_body"`
	CSSContent  string          `json:"css_content"`
	FontConfig  json.RawMessage `json:"font_config"`
	IsActive    bool            `json:"is_active"`
	CreatedBy   *string         `json:"created_by,omitempty"`
	UpdatedBy   *string         `json:"updated_by,omitempty"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// CreateTemplateVersionRequest is the payload for creating a new version.
type CreateTemplateVersionRequest struct {
	VersionName string          `json:"version_name"`
	HTMLBody    string          `json:"html_body"`
	CSSContent  string          `json:"css_content"`
	FontConfig  json.RawMessage `json:"font_config"`
	IsActive    bool            `json:"is_active"`
}

// UpdateTemplateVersionRequest is the payload for partial updates.
type UpdateTemplateVersionRequest struct {
	VersionName *string          `json:"version_name,omitempty"`
	HTMLBody    *string          `json:"html_body,omitempty"`
	CSSContent  *string          `json:"css_content,omitempty"`
	FontConfig  *json.RawMessage `json:"font_config,omitempty"`
}

// allowedFontURLPrefixes restricts font URLs to trusted Google Fonts origins.
var allowedFontURLPrefixes = []string{
	"https://fonts.googleapis.com/",
	"https://fonts.gstatic.com/",
}

// validateFontConfig checks that all font URLs in the config use allowed origins.
// Returns an error message if any URL is invalid, or empty string if valid.
func validateFontConfig(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return ""
	}
	var fonts map[string]struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &fonts); err != nil {
		return adminMsg.InvalidFontConfig
	}
	for _, fc := range fonts {
		if fc.URL == "" {
			continue
		}
		allowed := false
		for _, prefix := range allowedFontURLPrefixes {
			if strings.HasPrefix(fc.URL, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return adminMsg.FontURLNotTrusted
		}
	}
	return ""
}

const maxVersionsPerSlug = 5

// handleListTemplateVersions lists all versions for a given template slug.
// GET /api/admin/email-templates/{slug}/versions
func (a *App) handleListTemplateVersions(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	ctx := r.Context()

	rowsResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT id, slug, version_name, is_active, font_config,
				       created_by::text, updated_by::text, created_at, updated_at
				FROM email_template_versions
				WHERE slug = $1
				ORDER BY is_active DESC, created_at DESC
			`, slug)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query template versions", zap.String("slug", slug), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := rowsResult.(*sql.Rows)
	defer rows.Close()

	var versions []TemplateVersionListItem
	for rows.Next() {
		var v TemplateVersionListItem
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&v.ID, &v.Slug, &v.VersionName, &v.IsActive, &v.FontConfig,
			&v.CreatedBy, &v.UpdatedBy, &createdAt, &updatedAt); err != nil {
			a.log().Error("Failed to scan template version", zap.Error(err))
			continue
		}
		v.CreatedAt = createdAt.Format(time.RFC3339)
		v.UpdatedAt = updatedAt.Format(time.RFC3339)
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate template versions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if versions == nil {
		versions = []TemplateVersionListItem{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"versions":     versions,
		"slug":         slug,
		"max_versions": maxVersionsPerSlug,
	})
}

// handleCreateTemplateVersion creates a new version for a template slug.
// POST /api/admin/email-templates/{slug}/versions
func (a *App) handleCreateTemplateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	var req CreateTemplateVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Validate required fields
	if strings.TrimSpace(req.VersionName) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.VersionNameRequired})
		return
	}
	if strings.TrimSpace(req.HTMLBody) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.HTMLBodyRequired})
		return
	}
	if req.FontConfig == nil {
		req.FontConfig = json.RawMessage(`{}`)
	}
	if errMsg := validateFontConfig(req.FontConfig); errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check slug exists in email_templates
	var slugExists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM email_templates WHERE slug = $1)`, slug).Scan(&slugExists)
	if err != nil {
		a.log().Error("Failed to check template existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !slugExists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	// Count existing versions
	var versionCount int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM email_template_versions WHERE slug = $1`, slug).Scan(&versionCount)
	if err != nil {
		a.log().Error("Failed to count template versions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if versionCount >= maxVersionsPerSlug {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.MaxVersionsReached})
		return
	}

	// If is_active=true, deactivate other versions first
	if req.IsActive {
		_, err = tx.ExecContext(ctx,
			`UPDATE email_template_versions SET is_active = false WHERE slug = $1 AND is_active = true`, slug)
		if err != nil {
			a.log().Error("Failed to deactivate existing versions", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Insert the new version
	var created TemplateVersionDetail
	var createdAt, updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO email_template_versions (slug, version_name, html_body, css_content, font_config, is_active, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id, slug, version_name, html_body, css_content, font_config, is_active,
		          created_by::text, updated_by::text, created_at, updated_at
	`, slug, req.VersionName, req.HTMLBody, req.CSSContent, req.FontConfig, req.IsActive, actorUserID).Scan(
		&created.ID, &created.Slug, &created.VersionName, &created.HTMLBody, &created.CSSContent,
		&created.FontConfig, &created.IsActive, &created.CreatedBy, &created.UpdatedBy,
		&createdAt, &updatedAt,
	)
	if err != nil {
		a.log().Error("Failed to create template version", zap.String("slug", slug), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	created.CreatedAt = createdAt.Format(time.RFC3339)
	created.UpdatedAt = updatedAt.Format(time.RFC3339)

	// If the version was set as active, update the parent template's html_content
	if req.IsActive {
		composedHTML := notification.ComposeEmailHTMLFromJSON(created.HTMLBody, created.CSSContent, created.FontConfig)
		_, err = tx.ExecContext(ctx, `
			UPDATE email_templates SET html_content = $1, updated_by = $2, updated_at = NOW()
			WHERE slug = $3
		`, composedHTML, actorUserID, slug)
		if err != nil {
			a.log().Error("Failed to update parent template", zap.String("slug", slug), zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"slug":         slug,
		"version_id":   created.ID,
		"version_name": req.VersionName,
		"is_active":    req.IsActive,
	})
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "email_template_version.created", "email_template_version", created.ID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleGetTemplateVersion returns the full detail of a single version.
// GET /api/admin/email-templates/{slug}/versions/{versionId}
func (a *App) handleGetTemplateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	versionID := chi.URLParam(r, "versionId")
	if slug == "" || versionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	ctx := r.Context()

	var v TemplateVersionDetail
	var createdAt, updatedAt time.Time
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT id, slug, version_name, html_body, css_content, font_config, is_active,
			       created_by::text, updated_by::text, created_at, updated_at
			FROM email_template_versions
			WHERE id = $1 AND slug = $2
		`, versionID, slug).Scan(
			&v.ID, &v.Slug, &v.VersionName, &v.HTMLBody, &v.CSSContent,
			&v.FontConfig, &v.IsActive, &v.CreatedBy, &v.UpdatedBy,
			&createdAt, &updatedAt,
		)
	})
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.VersionNotFound})
		return
	}
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query template version", zap.String("slug", slug), zap.String("versionId", versionID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	v.CreatedAt = createdAt.Format(time.RFC3339)
	v.UpdatedAt = updatedAt.Format(time.RFC3339)

	writeJSON(w, http.StatusOK, v)
}

// handleUpdateTemplateVersion performs a partial update on a version.
// PUT /api/admin/email-templates/{slug}/versions/{versionId}
func (a *App) handleUpdateTemplateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	versionID := chi.URLParam(r, "versionId")
	if slug == "" || versionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	var req UpdateTemplateVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Check at least one field is being updated
	if req.VersionName == nil && req.HTMLBody == nil && req.CSSContent == nil && req.FontConfig == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}
	if req.FontConfig != nil {
		if errMsg := validateFontConfig(*req.FontConfig); errMsg != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": errMsg})
			return
		}
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check the version exists
	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM email_template_versions WHERE id = $1 AND slug = $2)`,
		versionID, slug).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check version existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.VersionNotFound})
		return
	}

	// Build dynamic update
	setClauses := []string{"updated_by = $1", "updated_at = NOW()"}
	args := []interface{}{actorUserID}
	argIdx := 2

	if req.VersionName != nil {
		setClauses = append(setClauses, fmt.Sprintf("version_name = $%d", argIdx))
		args = append(args, *req.VersionName)
		argIdx++
	}
	if req.HTMLBody != nil {
		setClauses = append(setClauses, fmt.Sprintf("html_body = $%d", argIdx))
		args = append(args, *req.HTMLBody)
		argIdx++
	}
	if req.CSSContent != nil {
		setClauses = append(setClauses, fmt.Sprintf("css_content = $%d", argIdx))
		args = append(args, *req.CSSContent)
		argIdx++
	}
	if req.FontConfig != nil {
		setClauses = append(setClauses, fmt.Sprintf("font_config = $%d", argIdx))
		args = append(args, *req.FontConfig)
		argIdx++
	}

	args = append(args, versionID, slug)
	query := fmt.Sprintf(`
		UPDATE email_template_versions
		SET %s
		WHERE id = $%d AND slug = $%d
		RETURNING id, slug, version_name, html_body, css_content, font_config, is_active,
		          created_by::text, updated_by::text, created_at, updated_at
	`, strings.Join(setClauses, ", "), argIdx, argIdx+1)

	var updated TemplateVersionDetail
	var createdAt, updatedAt time.Time
	err = tx.QueryRowContext(ctx, query, args...).Scan(
		&updated.ID, &updated.Slug, &updated.VersionName, &updated.HTMLBody, &updated.CSSContent,
		&updated.FontConfig, &updated.IsActive, &updated.CreatedBy, &updated.UpdatedBy,
		&createdAt, &updatedAt,
	)
	if err != nil {
		a.log().Error("Failed to update template version", zap.String("versionId", versionID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	updated.CreatedAt = createdAt.Format(time.RFC3339)
	updated.UpdatedAt = updatedAt.Format(time.RFC3339)

	// If this version is active, also update the parent template
	if updated.IsActive {
		composedHTML := notification.ComposeEmailHTMLFromJSON(updated.HTMLBody, updated.CSSContent, updated.FontConfig)
		_, err = tx.ExecContext(ctx, `
			UPDATE email_templates SET html_content = $1, updated_by = $2, updated_at = NOW()
			WHERE slug = $3
		`, composedHTML, actorUserID, slug)
		if err != nil {
			a.log().Error("Failed to update parent template", zap.String("slug", slug), zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"slug":       slug,
		"version_id": versionID,
		"action":     "updated",
	})
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "email_template_version.updated", "email_template_version", versionID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteTemplateVersion deletes a non-active version.
// DELETE /api/admin/email-templates/{slug}/versions/{versionId}
func (a *App) handleDeleteTemplateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	versionID := chi.URLParam(r, "versionId")
	if slug == "" || versionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check if version exists and whether it is active
	var isActive bool
	err = tx.QueryRowContext(ctx,
		`SELECT is_active FROM email_template_versions WHERE id = $1 AND slug = $2`,
		versionID, slug).Scan(&isActive)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.VersionNotFound})
		return
	}
	if err != nil {
		a.log().Error("Failed to query template version", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if isActive {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.CannotDeleteActive})
		return
	}

	// Delete the version
	_, err = tx.ExecContext(ctx,
		`DELETE FROM email_template_versions WHERE id = $1 AND slug = $2`,
		versionID, slug)
	if err != nil {
		a.log().Error("Failed to delete template version", zap.String("versionId", versionID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"slug":       slug,
		"version_id": versionID,
		"action":     "deleted",
	})
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "email_template_version.deleted", "email_template_version", versionID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.VersionDeleted})
}

// handleActivateTemplateVersion activates a version and deactivates others for the same slug.
// POST /api/admin/email-templates/{slug}/versions/{versionId}/activate
func (a *App) handleActivateTemplateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	versionID := chi.URLParam(r, "versionId")
	if slug == "" || versionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check the version exists
	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM email_template_versions WHERE id = $1 AND slug = $2)`,
		versionID, slug).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check version existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.VersionNotFound})
		return
	}

	// Deactivate all versions for this slug
	_, err = tx.ExecContext(ctx,
		`UPDATE email_template_versions SET is_active = false WHERE slug = $1 AND is_active = true`, slug)
	if err != nil {
		a.log().Error("Failed to deactivate versions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Activate the target version
	var activated TemplateVersionDetail
	var createdAt, updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE email_template_versions
		SET is_active = true, updated_by = $1, updated_at = NOW()
		WHERE id = $2 AND slug = $3
		RETURNING id, slug, version_name, html_body, css_content, font_config, is_active,
		          created_by::text, updated_by::text, created_at, updated_at
	`, actorUserID, versionID, slug).Scan(
		&activated.ID, &activated.Slug, &activated.VersionName, &activated.HTMLBody, &activated.CSSContent,
		&activated.FontConfig, &activated.IsActive, &activated.CreatedBy, &activated.UpdatedBy,
		&createdAt, &updatedAt,
	)
	if err != nil {
		a.log().Error("Failed to activate template version", zap.String("versionId", versionID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	activated.CreatedAt = createdAt.Format(time.RFC3339)
	activated.UpdatedAt = updatedAt.Format(time.RFC3339)

	// Update the parent email_templates.html_content with the composed full HTML
	composedHTML := notification.ComposeEmailHTMLFromJSON(activated.HTMLBody, activated.CSSContent, activated.FontConfig)
	_, err = tx.ExecContext(ctx, `
		UPDATE email_templates SET html_content = $1, updated_by = $2, updated_at = NOW()
		WHERE slug = $3
	`, composedHTML, actorUserID, slug)
	if err != nil {
		a.log().Error("Failed to update parent template", zap.String("slug", slug), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"slug":         slug,
		"version_id":   versionID,
		"version_name": activated.VersionName,
		"action":       "activated",
	})
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "email_template_version.activated", "email_template_version", versionID, payloadJSON,
	)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, activated)
}

// handlePreviewTemplateVersion composes and renders a version with sample data.
// POST /api/admin/email-templates/{slug}/versions/{versionId}/preview
func (a *App) handlePreviewTemplateVersion(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	versionID := chi.URLParam(r, "versionId")
	if slug == "" || versionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	ctx := r.Context()

	// Fetch version
	var htmlBody, cssContent string
	var fontConfig json.RawMessage
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT html_body, css_content, font_config
			FROM email_template_versions
			WHERE id = $1 AND slug = $2
		`, versionID, slug).Scan(&htmlBody, &cssContent, &fontConfig)
	})
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.VersionNotFound})
		return
	}
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query template version", zap.String("slug", slug), zap.String("versionId", versionID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Compose the full HTML
	composedHTML := notification.ComposeEmailHTMLFromJSON(htmlBody, cssContent, fontConfig)

	// Use existing template renderer to inject sample variables
	if a.emailNotifier == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.EmailNotConfigured})
		return
	}

	renderedHTML, err := a.emailNotifier.RenderTemplatePreview(ctx, slug, composedHTML)
	if err != nil {
		a.log().Error("Failed to render template preview",
			zap.String("slug", slug),
			zap.String("request_id", r.Header.Get("X-Request-ID")),
			zap.Error(err))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidRequest})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"rendered_html": renderedHTML})
}
