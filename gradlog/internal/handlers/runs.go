// Package handlers contains HTTP request handlers for the Gradlog API.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/models"
)

// RunHandler handles run-related requests.
type RunHandler struct {
	db             *database.DB
	projectHandler *ProjectHandler
}

// NewRunHandler creates a new run handler.
func NewRunHandler(db *database.DB, projectHandler *ProjectHandler) *RunHandler {
	return &RunHandler{
		db:             db,
		projectHandler: projectHandler,
	}
}

// CreateRunRequest is the request body for creating a new run.
type CreateRunRequest struct {
	Name   *string                `json:"name"`
	Params map[string]interface{} `json:"params"`
	Tags   map[string]interface{} `json:"tags"`
}

// CreateRun creates a new run within an experiment.
// POST /api/v1/experiments/:experimentId/runs
func (h *RunHandler) CreateRun(c *gin.Context) {
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid experiment ID",
		})
		return
	}

	// Get the project ID for access check.
	var projectID uuid.UUID
	err = h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT project_id FROM experiments WHERE id = $1
	`, experimentID).Scan(&projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Experiment not found",
		})
		return
	}

	userID := middleware.GetUserIDFromContext(c)
	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have permission to create runs in this experiment",
		})
		return
	}

	var req CreateRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body for simple run creation.
		req = CreateRunRequest{}
	}

	// Default empty maps for JSONB fields.
	if req.Params == nil {
		req.Params = make(map[string]interface{})
	}
	if req.Tags == nil {
		req.Tags = make(map[string]interface{})
	}

	paramsJSON, _ := json.Marshal(req.Params)
	tagsJSON, _ := json.Marshal(req.Tags)

	var run models.Run
	err = h.db.Pool.QueryRow(c.Request.Context(), `
		INSERT INTO runs (experiment_id, name, status, params, tags)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, experiment_id, name, status, params, tags, start_time, end_time, created_at, updated_at
	`, experimentID, req.Name, models.RunStatusRunning, paramsJSON, tagsJSON).Scan(
		&run.ID, &run.ExperimentID, &run.Name, &run.Status,
		&paramsJSON, &tagsJSON,
		&run.StartTime, &run.EndTime, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create run",
		})
		return
	}

	json.Unmarshal(paramsJSON, &run.Params)
	json.Unmarshal(tagsJSON, &run.Tags)

	c.JSON(http.StatusCreated, run)
}

// ListRuns returns all runs in an experiment.
// GET /api/v1/experiments/:experimentId/runs
func (h *RunHandler) ListRuns(c *gin.Context) {
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid experiment ID",
		})
		return
	}

	// Get the project ID for access check.
	var projectID uuid.UUID
	err = h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT project_id FROM experiments WHERE id = $1
	`, experimentID).Scan(&projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Experiment not found",
		})
		return
	}

	userID := middleware.GetUserIDFromContext(c)
	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Experiment not found",
		})
		return
	}

	rows, err := h.db.Pool.Query(c.Request.Context(), `
		SELECT id, experiment_id, name, status, params, tags, start_time, end_time, created_at, updated_at
		FROM runs
		WHERE experiment_id = $1
		ORDER BY created_at DESC
	`, experimentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch runs",
		})
		return
	}
	defer rows.Close()

	var runs []models.Run
	for rows.Next() {
		var run models.Run
		var paramsJSON, tagsJSON []byte
		if err := rows.Scan(
			&run.ID, &run.ExperimentID, &run.Name, &run.Status,
			&paramsJSON, &tagsJSON,
			&run.StartTime, &run.EndTime, &run.CreatedAt, &run.UpdatedAt,
		); err != nil {
			continue
		}
		json.Unmarshal(paramsJSON, &run.Params)
		json.Unmarshal(tagsJSON, &run.Tags)
		runs = append(runs, run)
	}

	if runs == nil {
		runs = []models.Run{}
	}

	c.JSON(http.StatusOK, runs)
}

// GetRun returns a single run by ID.
// GET /api/v1/runs/:id
func (h *RunHandler) GetRun(c *gin.Context) {
	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	var run models.Run
	var paramsJSON, tagsJSON []byte
	err = h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT id, experiment_id, name, status, params, tags, start_time, end_time, created_at, updated_at
		FROM runs WHERE id = $1
	`, runID).Scan(
		&run.ID, &run.ExperimentID, &run.Name, &run.Status,
		&paramsJSON, &tagsJSON,
		&run.StartTime, &run.EndTime, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Run not found",
		})
		return
	}

	json.Unmarshal(paramsJSON, &run.Params)
	json.Unmarshal(tagsJSON, &run.Tags)

	// Check access via experiment -> project.
	var projectID uuid.UUID
	h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT project_id FROM experiments WHERE id = $1
	`, run.ExperimentID).Scan(&projectID)

	userID := middleware.GetUserIDFromContext(c)
	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Run not found",
		})
		return
	}

	c.JSON(http.StatusOK, run)
}

// UpdateRunRequest is the request body for updating a run.
type UpdateRunRequest struct {
	Name   *string                `json:"name"`
	Status *string                `json:"status"`
	Params map[string]interface{} `json:"params"`
	Tags   map[string]interface{} `json:"tags"`
}

// UpdateRun updates a run.
// PATCH /api/v1/runs/:id
func (h *RunHandler) UpdateRun(c *gin.Context) {
	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	// Get run and check access.
	var experimentID uuid.UUID
	err = h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT experiment_id FROM runs WHERE id = $1
	`, runID).Scan(&experimentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Run not found",
		})
		return
	}

	var projectID uuid.UUID
	h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT project_id FROM experiments WHERE id = $1
	`, experimentID).Scan(&projectID)

	userID := middleware.GetUserIDFromContext(c)
	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have permission to update this run",
		})
		return
	}

	var req UpdateRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request body",
		})
		return
	}

	// Validate status if provided.
	if req.Status != nil {
		validStatuses := map[string]bool{
			models.RunStatusRunning:   true,
			models.RunStatusCompleted: true,
			models.RunStatusFailed:    true,
			models.RunStatusKilled:    true,
		}
		if !validStatuses[*req.Status] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid status. Must be running, completed, failed, or killed",
			})
			return
		}
	}

	// Build update query dynamically.
	var run models.Run
	var paramsJSON, tagsJSON []byte

	// Get current values first.
	h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT params, tags FROM runs WHERE id = $1
	`, runID).Scan(&paramsJSON, &tagsJSON)

	var currentParams, currentTags map[string]interface{}
	json.Unmarshal(paramsJSON, &currentParams)
	json.Unmarshal(tagsJSON, &currentTags)

	// Merge params if provided.
	if req.Params != nil {
		if currentParams == nil {
			currentParams = make(map[string]interface{})
		}
		for k, v := range req.Params {
			currentParams[k] = v
		}
	}

	// Merge tags if provided.
	if req.Tags != nil {
		if currentTags == nil {
			currentTags = make(map[string]interface{})
		}
		for k, v := range req.Tags {
			currentTags[k] = v
		}
	}

	paramsJSON, _ = json.Marshal(currentParams)
	tagsJSON, _ = json.Marshal(currentTags)

	// Update with end_time if status is terminal.
	var endTimeUpdate string
	if req.Status != nil && (*req.Status == models.RunStatusCompleted || *req.Status == models.RunStatusFailed || *req.Status == models.RunStatusKilled) {
		endTimeUpdate = ", end_time = NOW()"
	}

	query := `
		UPDATE runs
		SET name = COALESCE($1, name),
		    status = COALESCE($2, status),
		    params = $3,
		    tags = $4
		    ` + endTimeUpdate + `
		WHERE id = $5
		RETURNING id, experiment_id, name, status, params, tags, start_time, end_time, created_at, updated_at
	`

	err = h.db.Pool.QueryRow(c.Request.Context(), query,
		req.Name, req.Status, paramsJSON, tagsJSON, runID,
	).Scan(
		&run.ID, &run.ExperimentID, &run.Name, &run.Status,
		&paramsJSON, &tagsJSON,
		&run.StartTime, &run.EndTime, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update run",
		})
		return
	}

	json.Unmarshal(paramsJSON, &run.Params)
	json.Unmarshal(tagsJSON, &run.Tags)

	c.JSON(http.StatusOK, run)
}

// DeleteRun deletes a run and all associated data.
// DELETE /api/v1/runs/:id
func (h *RunHandler) DeleteRun(c *gin.Context) {
	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run ID",
		})
		return
	}

	// Get run and check access.
	var experimentID uuid.UUID
	err = h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT experiment_id FROM runs WHERE id = $1
	`, runID).Scan(&experimentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Run not found",
		})
		return
	}

	var projectID uuid.UUID
	h.db.Pool.QueryRow(c.Request.Context(), `
		SELECT project_id FROM experiments WHERE id = $1
	`, experimentID).Scan(&projectID)

	userID := middleware.GetUserIDFromContext(c)
	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "You don't have permission to delete this run",
		})
		return
	}

	_, err = h.db.Pool.Exec(c.Request.Context(), `
		DELETE FROM runs WHERE id = $1
	`, runID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete run",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
