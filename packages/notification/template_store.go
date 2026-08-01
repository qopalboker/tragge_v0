package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// ActiveTemplateVersion holds the components of an active email template version.
type ActiveTemplateVersion struct {
	HTMLBody   string
	CSSContent string
	FontConfig json.RawMessage
}

// TemplateOverrideStore defines the interface for fetching custom template overrides.
type TemplateOverrideStore interface {
	// GetTemplate retrieves a custom template override by slug.
	// Returns the HTML content if a custom template exists, or found=false for default.
	GetTemplate(ctx context.Context, slug string) (htmlContent string, found bool, err error)

	// GetActiveVersion retrieves the active template version for a slug from email_template_versions.
	// Returns nil if no active version exists.
	GetActiveVersion(ctx context.Context, slug string) (*ActiveTemplateVersion, error)
}

// EmailTemplate represents an email template record from the database.
type EmailTemplate struct {
	Slug        string
	Subject     *string
	HTMLContent *string
	Description *string
	Variables   *string
	UpdatedBy   *string
	UpdatedAt   time.Time
	CreatedAt   time.Time
}

// EmailTemplateListItem represents a template in the list response.
type EmailTemplateListItem struct {
	Slug        string     `json:"slug"`
	Description string     `json:"description"`
	Variables   string     `json:"variables"`
	HasCustom   bool       `json:"has_custom"`
	UpdatedAt   time.Time  `json:"updated_at"`
	UpdatedBy   *string    `json:"updated_by,omitempty"`
}

// EmailTemplateDetail represents detailed template information.
type EmailTemplateDetail struct {
	Slug        string    `json:"slug"`
	Subject     string    `json:"subject,omitempty"`
	Description string    `json:"description"`
	Variables   string    `json:"variables"`
	HTMLContent string    `json:"html_content"`
	IsDefault   bool      `json:"is_default"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   *string   `json:"updated_by,omitempty"`
}

// DBTemplateStore implements TemplateOverrideStore using a SQL database.
type DBTemplateStore struct {
	db *sql.DB
}

// NewDBTemplateStore creates a new database-backed template store.
func NewDBTemplateStore(db *sql.DB) *DBTemplateStore {
	return &DBTemplateStore{db: db}
}

// GetTemplate retrieves a custom template override by slug.
func (s *DBTemplateStore) GetTemplate(ctx context.Context, slug string) (string, bool, error) {
	var htmlContent sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT html_content
		FROM email_templates
		WHERE slug = $1
		AND html_content IS NOT NULL
		AND html_content != ''
	`, slug).Scan(&htmlContent)

	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	if !htmlContent.Valid || htmlContent.String == "" {
		return "", false, nil
	}

	return htmlContent.String, true, nil
}

// GetActiveVersion retrieves the active template version for a slug from email_template_versions.
func (s *DBTemplateStore) GetActiveVersion(ctx context.Context, slug string) (*ActiveTemplateVersion, error) {
	var v ActiveTemplateVersion

	err := s.db.QueryRowContext(ctx, `
		SELECT html_body, css_content, font_config
		FROM email_template_versions
		WHERE slug = $1 AND is_active = true
	`, slug).Scan(&v.HTMLBody, &v.CSSContent, &v.FontConfig)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &v, nil
}

// ListTemplates returns all registered email templates.
func (s *DBTemplateStore) ListTemplates(ctx context.Context) ([]EmailTemplateListItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			slug,
			COALESCE(description, ''),
			COALESCE(variables, ''),
			CASE WHEN html_content IS NOT NULL AND html_content != '' THEN true ELSE false END as has_custom,
			updated_at,
			updated_by
		FROM email_templates
		ORDER BY slug
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []EmailTemplateListItem
	for rows.Next() {
		var t EmailTemplateListItem
		if err := rows.Scan(&t.Slug, &t.Description, &t.Variables, &t.HasCustom, &t.UpdatedAt, &t.UpdatedBy); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}

	return templates, rows.Err()
}

// GetTemplateDetail returns detailed information about a specific template.
func (s *DBTemplateStore) GetTemplateDetail(ctx context.Context, slug string) (*EmailTemplateDetail, error) {
	var t EmailTemplateDetail
	var subject, htmlContent sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT
			slug,
			subject,
			COALESCE(description, ''),
			COALESCE(variables, ''),
			html_content,
			updated_at,
			updated_by
		FROM email_templates
		WHERE slug = $1
	`, slug).Scan(&t.Slug, &subject, &t.Description, &t.Variables, &htmlContent, &t.UpdatedAt, &t.UpdatedBy)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if subject.Valid {
		t.Subject = subject.String
	}

	if htmlContent.Valid && htmlContent.String != "" {
		t.HTMLContent = htmlContent.String
		t.IsDefault = false
	} else {
		t.IsDefault = true
	}

	return &t, nil
}

// UpdateTemplate updates a template's HTML content.
func (s *DBTemplateStore) UpdateTemplate(ctx context.Context, slug, htmlContent, updatedBy string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE email_templates
		SET html_content = $1, updated_by = $2, updated_at = NOW()
		WHERE slug = $3
	`, htmlContent, updatedBy, slug)

	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// ResetTemplate resets a template to use the default embedded version.
func (s *DBTemplateStore) ResetTemplate(ctx context.Context, slug, updatedBy string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE email_templates
		SET html_content = NULL, updated_by = $1, updated_at = NOW()
		WHERE slug = $2
	`, updatedBy, slug)

	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// TemplateExists checks if a template with the given slug exists.
func (s *DBTemplateStore) TemplateExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM email_templates WHERE slug = $1)
	`, slug).Scan(&exists)

	return exists, err
}
