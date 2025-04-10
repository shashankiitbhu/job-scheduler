package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type JobResponse struct {
	ID            string     `json:"id"`
	Command       string     `json:"command"`
	Type          string     `json:"type"`
	Status        string     `json:"status"`
	Schedule      string     `json:"schedule,omitempty"`
	RunAt         *time.Time `json:"run_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     time.Time  `json:"started_at,omitempty"`
	FinishedAt    time.Time  `json:"finished_at,omitempty"`
	Output        string     `json:"output,omitempty"`
	Error         string     `json:"error,omitempty"`
	RetryCount    int        `json:"retry_count"`
	MaxRetries    int        `json:"max_retries"`
	RetryDelay    int        `json:"retry_delay"`
	NextRetryAt   *time.Time `json:"next_retry_at,omitempty"`
	OriginalJobID string     `json:"original_job_id,omitempty"`
}

func (s *Server) getJob(c *gin.Context) {
	id := c.Param("id")
	job, err := s.jobService.GetJob(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	response := JobResponse{
		ID:            job.ID,
		Command:       job.Command,
		Type:          string(job.Type),
		Status:        string(job.Status),
		Schedule:      job.Schedule,
		CreatedAt:     job.CreatedAt,
		StartedAt:     job.StartedAt,
		FinishedAt:    job.FinishedAt,
		Output:        job.Output,
		Error:         job.Error,
		RetryCount:    job.RetryCount,
		MaxRetries:    job.MaxRetries,
		RetryDelay:    job.RetryDelay,
		OriginalJobID: job.OriginalJobID,
	}

	if !job.RunAt.IsZero() {
		response.RunAt = &job.RunAt
	}

	if !job.NextRetryAt.IsZero() {
		response.NextRetryAt = &job.NextRetryAt
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) listJobs(c *gin.Context) {
	jobs := s.jobService.GetAllJobs()

	response := make([]JobResponse, 0, len(jobs))
	for _, job := range jobs {
		jobResp := JobResponse{
			ID:            job.ID,
			Command:       job.Command,
			Type:          string(job.Type),
			Status:        string(job.Status),
			Schedule:      job.Schedule,
			CreatedAt:     job.CreatedAt,
			StartedAt:     job.StartedAt,
			FinishedAt:    job.FinishedAt,
			Output:        job.Output,
			Error:         job.Error,
			RetryCount:    job.RetryCount,
			MaxRetries:    job.MaxRetries,
			RetryDelay:    job.RetryDelay,
			OriginalJobID: job.OriginalJobID,
		}

		if !job.RunAt.IsZero() {
			jobResp.RunAt = &job.RunAt
		}

		if !job.NextRetryAt.IsZero() {
			jobResp.NextRetryAt = &job.NextRetryAt
		}

		response = append(response, jobResp)
	}

	c.JSON(http.StatusOK, response)
}

type Server struct {
	jobService *JobService
	scheduler  *Scheduler
	port       int
}

func NewServer(jobService *JobService, scheduler *Scheduler, port int) *Server {
	return &Server{
		jobService: jobService,
		scheduler:  scheduler,
		port:       port,
	}
}

func (s *Server) Start() error {
	router := gin.Default()

	router.Static("/static", "./static")

	router.LoadHTMLFiles("static/index.html")
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	api := router.Group("/api")
	{
		api.POST("/jobs", s.createJob)
		api.GET("/jobs", s.listJobs)
		api.GET("/jobs/:id", s.getJob)
		api.GET("/jobs/stats", s.getJobStats)
		api.POST("/jobs/:id/retry", s.retryJob) // New endpoint for retrying jobs
	}

	return router.Run(fmt.Sprintf(":%d", s.port))
}

func (s *Server) createJob(c *gin.Context) {
	var request struct {
		Command    string     `json:"command" binding:"required"`
		Type       string     `json:"type" binding:"required"`
		Schedule   string     `json:"schedule"`
		RunAt      *time.Time `json:"run_at"`
		MaxRetries int        `json:"max_retries"`
		RetryDelay int        `json:"retry_delay"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var jobType JobType
	switch request.Type {
	case string(JobTypeOnce):
		jobType = JobTypeOnce
	case string(JobTypeCron):
		jobType = JobTypeCron
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job type, must be 'once' or 'cron'"})
		return
	}

	var runAt time.Time
	if request.RunAt != nil {
		runAt = *request.RunAt
	}

	// Validate retry settings
	if request.MaxRetries < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_retries cannot be negative"})
		return
	}

	if request.RetryDelay < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "retry_delay cannot be negative"})
		return
	}

	job, err := s.jobService.CreateJob(
		request.Command,
		jobType,
		request.Schedule,
		runAt,
		request.MaxRetries,
		request.RetryDelay,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if jobType == JobTypeCron && s.scheduler != nil {
		if err := s.scheduler.ScheduleJob(job); err != nil {
			fmt.Printf("Failed to schedule cron job: %v", err)
		}
	}

	c.JSON(http.StatusCreated, job)
}

// Add a retry endpoint
func (s *Server) retryJob(c *gin.Context) {
	id := c.Param("id")

	retryJob, err := s.jobService.RetryJob(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, retryJob)
}

type JobStats struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

func (s *Server) getJobStats(c *gin.Context) {
	jobs := s.jobService.GetAllJobs()

	stats := JobStats{}
	for _, job := range jobs {
		switch job.Status {
		case JobStatusPending:
			stats.Pending++
		case JobStatusRunning:
			stats.Running++
		case JobStatusCompleted:
			stats.Completed++
		case JobStatusFailed:
			stats.Failed++
		}
	}

	c.JSON(http.StatusOK, stats)
}
