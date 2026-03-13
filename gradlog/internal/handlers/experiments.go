// Package handlers provides HTTP request handlers for the Gradlog API.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/models"
	"github.com/gradlog/gradlog/internal/storage"
)

// ExperimentHandler handles CRUD operations for experiments within projects.
type ExperimentHandler struct {
	db             *database.DB
	projectHandler *ProjectHandler
	storage        *storage.LocalStorage
}

// NewExperimentHandler creates a new ExperimentHandler.
func NewExperimentHandler(db *database.DB, ph *ProjectHandler, store *storage.LocalStorage) *ExperimentHandler {
	return &ExperimentHandler{db: db, projectHandler: ph, storage: store}
}

// CreateExperiment handles POST /projects/:id/experiments.
// Creates a new experiment within the specified project.
func (h *ExperimentHandler) CreateExperiment(c *gin.Context) {
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

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
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

	exp := &models.Experiment{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`INSERT INTO experiments (id, project_id, name, description, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())
		 RETURNING id, project_id, name, description, created_at, updated_at`,
		projectID, req.Name, req.Description,
	).Scan(&exp.ID, &exp.ProjectID, &exp.Name, &exp.Description, &exp.CreatedAt, &exp.UpdatedAt)
	if err != nil {
		if isDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "experiment name already exists in this project"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create experiment"})
		return
	}

	c.JSON(http.StatusCreated, exp)
}

// ListExperiments handles GET /projects/:id/experiments.
// Returns all experiments belonging to the specified project.
func (h *ExperimentHandler) ListExperiments(c *gin.Context) {
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

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT id, project_id, name, description, created_at, updated_at
		 FROM experiments WHERE project_id = $1 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list experiments"})
		return
	}
	defer rows.Close()

	experiments := make([]models.Experiment, 0)
	for rows.Next() {
		var exp models.Experiment
		if err := rows.Scan(&exp.ID, &exp.ProjectID, &exp.Name, &exp.Description,
			&exp.CreatedAt, &exp.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan experiment"})
			return
		}
		experiments = append(experiments, exp)
	}

	c.JSON(http.StatusOK, experiments)
}

// GetExperiment handles GET /experiments/:id.
// Returns a single experiment by its ID.
func (h *ExperimentHandler) GetExperiment(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid experiment id"})
		return
	}

	exp := &models.Experiment{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT id, project_id, name, description, created_at, updated_at
		 FROM experiments WHERE id = $1`,
		id,
	).Scan(&exp.ID, &exp.ProjectID, &exp.Name, &exp.Description, &exp.CreatedAt, &exp.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, exp.ProjectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	c.JSON(http.StatusOK, exp)
}

// UpdateExperiment handles PATCH /experiments/:id.
// Performs a partial update using COALESCE to preserve unset fields.
func (h *ExperimentHandler) UpdateExperiment(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid experiment id"})
		return
	}

	// Fetch the existing experiment to check project access.
	var projectID uuid.UUID
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT project_id FROM experiments WHERE id = $1`, id,
	).Scan(&projectID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
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

	exp := &models.Experiment{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`UPDATE experiments
		 SET name        = COALESCE($2, name),
		     description = COALESCE($3, description),
		     updated_at  = NOW()
		 WHERE id = $1
		 RETURNING id, project_id, name, description, created_at, updated_at`,
		id, req.Name, req.Description,
	).Scan(&exp.ID, &exp.ProjectID, &exp.Name, &exp.Description, &exp.CreatedAt, &exp.UpdatedAt)
	if err != nil {
		if isDuplicateKeyError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "experiment name already exists in this project"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update experiment"})
		return
	}

	c.JSON(http.StatusOK, exp)
}

// DeleteExperiment handles DELETE /experiments/:id.
// Deletes an experiment and all of its associated data (cascaded in the DB schema).
func (h *ExperimentHandler) DeleteExperiment(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid experiment id"})
		return
	}

	var projectID uuid.UUID
	if err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT project_id FROM experiments WHERE id = $1`, id,
	).Scan(&projectID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "experiment not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	paths := make([]string, 0)
	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT a.storage_path
		 FROM artifacts a
		 JOIN runs r ON r.id = a.run_id
		 WHERE r.experiment_id = $1
		 UNION
		 SELECT ac.storage_path
		 FROM artifact_chunks ac
		 JOIN artifacts a ON a.id = ac.artifact_id
		 JOIN runs r ON r.id = a.run_id
		 WHERE r.experiment_id = $1`,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare experiment deletion"})
		return
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare experiment deletion"})
			return
		}
		paths = append(paths, p)
	}
	rows.Close()

	if _, err := h.db.Pool.Exec(
		c.Request.Context(),
		`DELETE FROM experiments WHERE id = $1`, id,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete experiment"})
		return
	}

	if h.storage != nil {
		for _, p := range paths {
			h.storage.Delete(p)
		}
	}

	c.JSON(http.StatusNoContent, nil)
}
