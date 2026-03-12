package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gradlog/gradlog/internal/database"
	"github.com/gradlog/gradlog/internal/middleware"
	"github.com/gradlog/gradlog/internal/models"
)

// MetricHandler handles logging and retrieval of run metrics.
type MetricHandler struct {
	db             *database.DB
	projectHandler *ProjectHandler
}

// NewMetricHandler creates a new MetricHandler.
func NewMetricHandler(db *database.DB, ph *ProjectHandler) *MetricHandler {
	return &MetricHandler{db: db, projectHandler: ph}
}

// getProjectIDForRun returns the project ID associated with a run by traversing
// the run → experiment → project hierarchy.
func (h *MetricHandler) getProjectIDForRun(c *gin.Context, runID uuid.UUID) (uuid.UUID, error) {
	var projectID uuid.UUID
	err := h.db.Pool.QueryRow(
		c.Request.Context(),
		`SELECT e.project_id FROM runs r
		 JOIN experiments e ON e.id = r.experiment_id
		 WHERE r.id = $1`,
		runID,
	).Scan(&projectID)
	return projectID, err
}

// LogMetric handles POST /runs/:runId/metrics.
// Logs a single metric value for a run.
func (h *MetricHandler) LogMetric(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		Key       string     `json:"key" binding:"required"`
		Value     float64    `json:"value" binding:"required"`
		Step      int64      `json:"step"`
		Timestamp *time.Time `json:"timestamp"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ts := time.Now()
	if req.Timestamp != nil {
		ts = *req.Timestamp
	}

	metric := &models.Metric{}
	err = h.db.Pool.QueryRow(
		c.Request.Context(),
		`INSERT INTO metrics (id, run_id, key, value, step, timestamp)
		 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)
		 RETURNING id, run_id, key, value, step, timestamp`,
		runID, req.Key, req.Value, req.Step, ts,
	).Scan(&metric.ID, &metric.RunID, &metric.Key, &metric.Value, &metric.Step, &metric.Timestamp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to log metric"})
		return
	}

	c.JSON(http.StatusCreated, metric)
}

// LogMetricsBatch handles POST /runs/:runId/metrics/batch.
// Logs multiple metric values in a single atomic transaction.
func (h *MetricHandler) LogMetricsBatch(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userCanEdit(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		Metrics []struct {
			Key       string     `json:"key" binding:"required"`
			Value     float64    `json:"value"`
			Step      int64      `json:"step"`
			Timestamp *time.Time `json:"timestamp"`
		} `json:"metrics" binding:"required,min=1"`
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

	metrics := make([]models.Metric, 0, len(req.Metrics))
	for _, m := range req.Metrics {
		ts := time.Now()
		if m.Timestamp != nil {
			ts = *m.Timestamp
		}

		metric := models.Metric{}
		err := tx.QueryRow(
			c.Request.Context(),
			`INSERT INTO metrics (id, run_id, key, value, step, timestamp)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)
			 RETURNING id, run_id, key, value, step, timestamp`,
			runID, m.Key, m.Value, m.Step, ts,
		).Scan(&metric.ID, &metric.RunID, &metric.Key, &metric.Value, &metric.Step, &metric.Timestamp)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to insert metric"})
			return
		}
		metrics = append(metrics, metric)
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to commit transaction"})
		return
	}

	c.JSON(http.StatusCreated, metrics)
}

// MetricHistory groups all recorded values for a single metric key.
type MetricHistory struct {
	Key     string         `json:"key"`
	History []models.Metric `json:"history"`
}

// GetMetrics handles GET /runs/:runId/metrics.
// Returns all metrics for a run, grouped by key.
func (h *MetricHandler) GetMetrics(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT id, run_id, key, value, step, timestamp
		 FROM metrics WHERE run_id = $1 ORDER BY key, step ASC`,
		runID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get metrics"})
		return
	}
	defer rows.Close()

	// Group metrics by key.
	grouped := make(map[string]*MetricHistory)
	order := make([]string, 0)
	for rows.Next() {
		var m models.Metric
		if err := rows.Scan(&m.ID, &m.RunID, &m.Key, &m.Value, &m.Step, &m.Timestamp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan metric"})
			return
		}
		if _, exists := grouped[m.Key]; !exists {
			grouped[m.Key] = &MetricHistory{Key: m.Key, History: []models.Metric{}}
			order = append(order, m.Key)
		}
		grouped[m.Key].History = append(grouped[m.Key].History, m)
	}

	result := make([]MetricHistory, 0, len(order))
	for _, key := range order {
		result = append(result, *grouped[key])
	}

	c.JSON(http.StatusOK, result)
}

// GetLatestMetrics handles GET /runs/:runId/metrics/latest.
// Returns the most recent value (by step) for each distinct metric key.
func (h *MetricHandler) GetLatestMetrics(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT DISTINCT ON (key) id, run_id, key, value, step, timestamp
		 FROM metrics WHERE run_id = $1
		 ORDER BY key, step DESC`,
		runID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get latest metrics"})
		return
	}
	defer rows.Close()

	metrics := make([]models.Metric, 0)
	for rows.Next() {
		var m models.Metric
		if err := rows.Scan(&m.ID, &m.RunID, &m.Key, &m.Value, &m.Step, &m.Timestamp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan metric"})
			return
		}
		metrics = append(metrics, m)
	}

	c.JSON(http.StatusOK, metrics)
}

// GetMetricHistory handles GET /runs/:runId/metrics/:key/history.
// Returns the full history of a single metric key for a run.
func (h *MetricHandler) GetMetricHistory(c *gin.Context) {
	userID := middleware.GetUserIDFromContext(c)
	if userID == uuid.Nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	projectID, err := h.getProjectIDForRun(c, runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if !h.projectHandler.userHasAccess(c, projectID, userID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	key := c.Param("key")
	rows, err := h.db.Pool.Query(
		c.Request.Context(),
		`SELECT id, run_id, key, value, step, timestamp
		 FROM metrics WHERE run_id = $1 AND key = $2
		 ORDER BY step ASC`,
		runID, key,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get metric history"})
		return
	}
	defer rows.Close()

	history := make([]models.Metric, 0)
	for rows.Next() {
		var m models.Metric
		if err := rows.Scan(&m.ID, &m.RunID, &m.Key, &m.Value, &m.Step, &m.Timestamp); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan metric"})
			return
		}
		history = append(history, m)
	}

	c.JSON(http.StatusOK, MetricHistory{Key: key, History: history})
}
