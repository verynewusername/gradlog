package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/models"
	"github.com/gradlog/gradlog/internal/storage"
	"github.com/jackc/pgx/v5"
)

// ProjectHandler handles CRUD operations and membership management for projects.
type ProjectHandler struct {
	db      *database.DB
	storage *storage.LocalStorage
}

// NewProjectHandler creates a new ProjectHandler.
func NewProjectHandler(db *database.DB, store *storage.LocalStorage) *ProjectHandler {
	return &ProjectHandler{db: db, storage: store}
}

// userHasAccess returns true if the given user is a member of the project.
func (h *ProjectHandler) userHasAccess(c *gin.Context, projectID, userID uuid.UUID) bool {
	var count int
	err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&count)
	return err == nil && count > 0
}

// userCanEdit returns true if the user has member-level (or higher) access to edit the project.
func (h *ProjectHandler) userCanEdit(c *gin.Context, projectID, userID uuid.UUID) bool {
	var role string
	err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&role)
	if err != nil {
		return false
	}
	return role == "owner" || role == "admin" || role == "member"
}

// userIsAdmin returns true if the user has admin-level (or higher) access.
func (h *ProjectHandler) userIsAdmin(c *gin.Context, projectID, userID uuid.UUID) bool {
	var role string
	err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&role)
	if err != nil {
		return false
	}
	return role == "owner" || role == "admin"
}

// CreateProject handles POST /projects.
// Creates a new project and adds the creator as its owner in a single transaction.
func (h *ProjectHandler) CreateProject(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tx, err := h.db.Pool.Begin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to begin transaction"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	project := &models.Project{}
	err = tx.QueryRow(
		c.Request.Context(),
		`INSERT INTO projects (id, name, description, owner_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())
		 RETURNING id, name, description, owner_id, created_at, updated_at`,
		req.Name, req.Description, userID,
	).Scan(&project.ID, &project.Name, &project.Description, &project.OwnerID,
		&project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		if isDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "project name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	if _, err := tx.Exec(
		c.Request.Context(),
		`INSERT INTO project_members (project_id, user_id, role, created_at)
		 VALUES ($1, $2, 'owner', NOW())`,
		project.ID, userID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add project owner"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
		return
	}

	c.JSON(http.StatusCreated, project)
}

// ListProjects handles GET /projects.
// Returns all projects the authenticated user is a member of.
func (h *ProjectHandler) ListProjects(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT p.id, p.name, p.description, p.owner_id, p.created_at, p.updated_at
		 FROM projects p
		 JOIN project_members pm ON pm.project_id = p.id
		 WHERE pm.user_id = $1
		 ORDER BY p.created_at DESC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}
	defer rows.Close()

	projects := make([]models.Project, 0)
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan project"})
			return
		}
		projects = append(projects, p)
	}

	c.JSON(http.StatusOK, projects)
}

// GetProject handles GET /projects/:id.
// Returns a single project by its ID.
func (h *ProjectHandler) GetProject(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	if !h.userHasAccess(c, id, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	project := &models.Project{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT id, name, description, owner_id, created_at, updated_at
		 FROM projects WHERE id = $1`,
		id,
	).Scan(&project.ID, &project.Name, &project.Description, &project.OwnerID,
		&project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	c.JSON(http.StatusOK, project)
}

// UpdateProject handles PATCH /projects/:id.
// Performs a partial update; requires admin or owner role.
func (h *ProjectHandler) UpdateProject(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	if !h.userIsAdmin(c, id, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	project := &models.Project{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`UPDATE projects
		 SET name        = COALESCE($2, name),
		     description = COALESCE($3, description),
		     updated_at  = NOW()
		 WHERE id = $1
		 RETURNING id, name, description, owner_id, created_at, updated_at`,
		id, req.Name, req.Description,
	).Scan(&project.ID, &project.Name, &project.Description, &project.OwnerID,
		&project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		if isDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "project name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update project"})
		return
	}

	c.JSON(http.StatusOK, project)
}

// DeleteProject handles DELETE /projects/:id.
// Only the project owner may delete a project.
func (h *ProjectHandler) DeleteProject(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	var role string
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&role); err != nil || role != "owner" {
		c.JSON(http.StatusForbidden, gin.H{"error": "only the project owner can delete a project"})
		return
	}

	// Capture storage paths before DB cascade deletion so underlying files can be
	// removed from disk after the project is deleted.
	paths := make([]string, 0)
	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT a.storage_path
		 FROM artifacts a
		 JOIN runs r ON r.id = a.run_id
		 JOIN experiments e ON e.id = r.experiment_id
		 WHERE e.project_id = $1
		 UNION
		 SELECT ac.storage_path
		 FROM artifact_chunks ac
		 JOIN artifacts a ON a.id = ac.artifact_id
		 JOIN runs r ON r.id = a.run_id
		 JOIN experiments e ON e.id = r.experiment_id
		 WHERE e.project_id = $1`,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare project deletion"})
		return
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare project deletion"})
			return
		}
		paths = append(paths, path)
	}
	rows.Close()

	if _, err := h.db.Pool.Exec(
		c.Request.Context(),
		`DELETE FROM projects WHERE id = $1`, id,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}

	if h.storage != nil {
		for _, p := range paths {
			h.storage.Delete(p)
		}
	}

	c.JSON(http.StatusNoContent, nil)
}

// ProjectMemberWithUser extends ProjectMember with the member's user information.
type ProjectMemberWithUser struct {
	models.ProjectMember
	Email      string `json:"email"`
	Name       string `json:"name"`
	PictureURL string `json:"picture_url"`
}

// ListMembers handles GET /projects/:id/members.
// Returns all members of the project along with their user information.
func (h *ProjectHandler) ListMembers(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	if !h.userHasAccess(c, id, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT pm.project_id, pm.user_id, pm.role, pm.created_at,
		        u.email, u.name, u.picture_url
		 FROM project_members pm
		 JOIN users u ON u.id = pm.user_id
		 WHERE pm.project_id = $1
		 ORDER BY pm.created_at ASC`,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list members"})
		return
	}
	defer rows.Close()

	members := make([]ProjectMemberWithUser, 0)
	for rows.Next() {
		var m ProjectMemberWithUser
		if err := rows.Scan(&m.ProjectID, &m.UserID, &m.Role, &m.CreatedAt,
			&m.Email, &m.Name, &m.PictureURL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan member"})
			return
		}
		members = append(members, m)
	}

	c.JSON(http.StatusOK, members)
}

// AddMember handles POST /projects/:id/members.
// Adds a user to a project by their email address; requires admin or owner role.
// Uses ON CONFLICT to upsert the role if the user is already a member.
func (h *ProjectHandler) AddMember(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	if !h.userIsAdmin(c, id, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	role := strings.ToLower(req.Role)
	if role == "" {
		role = "member"
	}
	if role != "member" && role != "admin" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be 'member' or 'admin'"})
		return
	}

	// Look up the target user by email.
	var targetUserID uuid.UUID
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT id FROM users WHERE email = $1`, req.Email,
	).Scan(&targetUserID); err != nil {
		if err == pgx.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find user"})
		return
	}

	member := &models.ProjectMember{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`INSERT INTO project_members (project_id, user_id, role, created_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role
		 RETURNING project_id, user_id, role, created_at`,
		id, targetUserID, role,
	).Scan(&member.ProjectID, &member.UserID, &member.Role, &member.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add member"})
		return
	}

	c.JSON(http.StatusOK, member)
}

// RemoveMember handles DELETE /projects/:id/members/:userId.
// Removes a user from the project; requires admin or owner role.
// The project owner cannot be removed.
func (h *ProjectHandler) RemoveMember(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}

	targetUserID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if !h.userIsAdmin(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// Prevent removing the project owner.
	var targetRole string
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, targetUserID,
	).Scan(&targetRole); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	if targetRole == "owner" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot remove the project owner"})
		return
	}

	if _, err := h.db.Pool.Exec(
		c.Request.Context(),
		`DELETE FROM project_members WHERE project_id = $1 AND user_id = $2`,
		projectID, targetUserID,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove member"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// isDuplicateKeyError checks whether a PostgreSQL error is a unique constraint violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
