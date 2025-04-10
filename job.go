package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const JobsDataKey = "jobs:data"

type JobType string

const (
	JobTypeOnce JobType = "once"
	JobTypeCron JobType = "cron"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type Job struct {
	ID            string    `json:"id"`
	Command       string    `json:"command"`
	Type          JobType   `json:"type"`
	Schedule      string    `json:"schedule,omitempty"`
	RunAt         time.Time `json:"run_at,omitempty"`
	Status        JobStatus `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	FinishedAt    time.Time `json:"finished_at,omitempty"`
	Output        string    `json:"output,omitempty"`
	Error         string    `json:"error,omitempty"`
	RetryCount    int       `json:"retry_count"`               // Add retry count
	MaxRetries    int       `json:"max_retries"`               // Add maximum retries
	RetryDelay    int       `json:"retry_delay"`               // Delay in seconds between retries
	NextRetryAt   time.Time `json:"next_retry_at,omitempty"`   // When to retry next
	OriginalJobID string    `json:"original_job_id,omitempty"` // For tracking retries of the same job
}

type JobService struct {
	redis     *redis.Client
	jobs      map[string]*Job
	jobsMutex sync.RWMutex
}

const (
	JobsQueueKey = "jobs:queue"
)

func NewJobService(redisClient *redis.Client) *JobService {
	return &JobService{
		redis: redisClient,
		jobs:  make(map[string]*Job),
	}
}

func (s *JobService) CreateJob(cmd string, jobType JobType, schedule string, runAt time.Time, maxRetries, retryDelay int) (*Job, error) {
	if cmd == "" {
		return nil, errors.New("command cannot be empty")
	}

	if jobType == JobTypeCron && schedule == "" {
		return nil, errors.New("cron jobs require a schedule")
	}

	job := &Job{
		ID:         uuid.New().String(),
		Command:    cmd,
		Type:       jobType,
		Schedule:   schedule,
		RunAt:      runAt,
		Status:     JobStatusPending,
		CreatedAt:  time.Now(),
		RetryCount: 0,
		MaxRetries: maxRetries,
		RetryDelay: retryDelay,
	}

	s.jobsMutex.Lock()
	s.jobs[job.ID] = job
	s.jobsMutex.Unlock()

	jobJSON, err := json.Marshal(job)
	if err == nil {
		ctx := context.Background()
		s.redis.HSet(ctx, JobsDataKey, job.ID, jobJSON)
	}

	// Only queue the job immediately if it's a one-time job with no delay
	if jobType == JobTypeOnce && runAt.IsZero() {
		if err := s.QueueJob(job); err != nil {
			s.jobsMutex.Lock()
			delete(s.jobs, job.ID)
			s.jobsMutex.Unlock()
			return nil, err
		}
	}

	return job, nil
}

// CheckDelayedJobs checks for delayed jobs that are ready to run
func (s *JobService) CheckDelayedJobs() {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	now := time.Now()
	for _, job := range s.jobs {
		// Skip jobs that are not pending or don't have a RunAt time set
		if job.Status != JobStatusPending || job.RunAt.IsZero() {
			continue
		}

		// Queue the job if it's time to run
		if now.After(job.RunAt) {
			// Don't hold the mutex during queue operation
			// Make a copy of the job to avoid race conditions
			jobCopy := *job
			s.jobsMutex.Unlock()

			fmt.Printf("Queueing delayed job %s scheduled for %s\n", job.ID, job.RunAt)
			err := s.QueueJob(&jobCopy)
			if err != nil {
				fmt.Printf("Failed to queue delayed job %s: %v\n", job.ID, err)
			}

			s.jobsMutex.Lock()
			// Just to be safe, check if the job still exists
			if _, exists := s.jobs[job.ID]; exists {
				// Clear the RunAt field since it's been queued
				s.jobs[job.ID].RunAt = time.Time{}
			}
		}
	}
}

func (s *JobService) QueueJob(job *Job) error {

	jobJSON, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	ctx := context.Background()
	err = s.redis.RPush(ctx, JobsQueueKey, jobJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to push job to queue: %w", err)
	}

	return nil
}

func (s *JobService) GetJob(id string) (*Job, error) {
	// First try Redis
	ctx := context.Background()
	jobJSON, err := s.redis.HGet(ctx, "jobs:data", id).Result()
	if err == nil {
		var job Job
		if err := json.Unmarshal([]byte(jobJSON), &job); err == nil {
			// Update local cache
			s.jobsMutex.Lock()
			s.jobs[id] = &job
			s.jobsMutex.Unlock()
			return &job, nil
		}
	}

	// Fallback to memory
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()
	job, exists := s.jobs[id]
	if !exists {
		return nil, errors.New("job not found")
	}
	return job, nil
}

func (s *JobService) GetAllJobs() []*Job {
	s.jobsMutex.RLock()
	defer s.jobsMutex.RUnlock()

	// Get all jobs from Redis first
	ctx := context.Background()
	redisJobs, err := s.redis.HGetAll(ctx, JobsDataKey).Result()
	if err != nil {
		log.Printf("Failed to get jobs from Redis: %v", err)
		// Fallback to in-memory
		return s.getInMemoryJobs()
	}

	// Merge Redis and in-memory jobs
	jobs := make([]*Job, 0, len(redisJobs))
	for id, jobJSON := range redisJobs {
		var job Job
		if err := json.Unmarshal([]byte(jobJSON), &job); err == nil {
			// Update in-memory map if newer than our version
			if existing, ok := s.jobs[id]; !ok || job.CreatedAt.After(existing.CreatedAt) {
				s.jobs[id] = &job
			}
			jobs = append(jobs, &job)
		}
	}

	return jobs
}

func (s *JobService) getInMemoryJobs() []*Job {
	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (s *JobService) UpdateJobStatus(id string, status JobStatus, output, errMsg string) error {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	job, exists := s.jobs[id]
	if !exists {
		return errors.New("job not found")
	}

	// Update local copy
	job.Status = status
	job.Output = output
	job.Error = errMsg

	if status == JobStatusRunning && job.StartedAt.IsZero() {
		job.StartedAt = time.Now()
	}

	if status == JobStatusCompleted || status == JobStatusFailed {
		job.FinishedAt = time.Now()
	}

	// Persist to Redis
	jobJSON, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	ctx := context.Background()
	err = s.redis.HSet(ctx, "jobs:data", id, jobJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to save job to Redis: %w", err)
	}

	return nil
}

func (s *JobService) DequeueJob(ctx context.Context) (*Job, error) {
	result, err := s.redis.BLPop(ctx, 0, JobsQueueKey).Result()
	if err != nil {
		if err == context.Canceled {
			return nil, err
		}
		return nil, fmt.Errorf("failed to pop job from queue: %w", err)
	}

	if len(result) < 2 {
		return nil, errors.New("invalid job data from queue")
	}

	var job Job
	err = json.Unmarshal([]byte(result[1]), &job)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	if existingJob, exists := s.jobs[job.ID]; exists {
		return existingJob, nil
	}

	job.Status = JobStatusPending
	s.jobs[job.ID] = &job

	return &job, nil
}

func setupRedisClient(config *Config) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPass,
		DB:       config.RedisDB,
	})

	ctx := context.Background()
	_, err := client.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	return client
}

// RetryJob creates a retry for a failed job
func (s *JobService) RetryJob(jobID string) (*Job, error) {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	originalJob, exists := s.jobs[jobID]
	if !exists {
		return nil, errors.New("job not found")
	}

	// Only retry failed jobs that haven't reached max retries
	if originalJob.Status != JobStatusFailed {
		return nil, errors.New("only failed jobs can be retried")
	}

	if originalJob.RetryCount >= originalJob.MaxRetries && originalJob.MaxRetries > 0 {
		return nil, errors.New("maximum retry attempts reached")
	}

	// If this is already a retry, find the original job
	originalJobID := originalJob.OriginalJobID
	if originalJobID == "" {
		originalJobID = jobID // This is the original job
	}

	// Create a new job as a retry of the original
	retryJob := &Job{
		ID:            uuid.New().String(),
		Command:       originalJob.Command,
		Type:          JobTypeOnce, // Retries are always one-time
		Status:        JobStatusPending,
		CreatedAt:     time.Now(),
		RetryCount:    originalJob.RetryCount + 1,
		MaxRetries:    originalJob.MaxRetries,
		RetryDelay:    originalJob.RetryDelay,
		OriginalJobID: originalJobID,
	}

	// Calculate next retry time if delay is specified
	if originalJob.RetryDelay > 0 {
		retryJob.RunAt = time.Now().Add(time.Duration(originalJob.RetryDelay) * time.Second)
	}

	// Store the retry job
	s.jobs[retryJob.ID] = retryJob

	// Update Redis
	jobJSON, err := json.Marshal(retryJob)
	if err == nil {
		ctx := context.Background()
		s.redis.HSet(ctx, JobsDataKey, retryJob.ID, jobJSON)
	}

	// Queue immediately if no delay
	if retryJob.RunAt.IsZero() {
		if err := s.QueueJob(retryJob); err != nil {
			delete(s.jobs, retryJob.ID)
			return nil, err
		}
	}

	return retryJob, nil
}

// CheckRetryJobs checks for jobs that need to be retried
func (s *JobService) CheckRetryJobs() {
	s.jobsMutex.Lock()
	defer s.jobsMutex.Unlock()

	now := time.Now()
	for _, job := range s.jobs {
		// Skip jobs that are not failed or don't have scheduled retry
		if job.Status != JobStatusFailed || job.NextRetryAt.IsZero() || job.RetryCount >= job.MaxRetries {
			continue
		}

		// Queue the job if it's time to retry
		if now.After(job.NextRetryAt) {
			// Create a new job as a retry
			retryJob := &Job{
				ID:            uuid.New().String(),
				Command:       job.Command,
				Type:          JobTypeOnce,
				Status:        JobStatusPending,
				CreatedAt:     time.Now(),
				RetryCount:    job.RetryCount + 1,
				MaxRetries:    job.MaxRetries,
				RetryDelay:    job.RetryDelay,
				OriginalJobID: job.OriginalJobID,
			}

			if job.OriginalJobID == "" {
				retryJob.OriginalJobID = job.ID
			}

			// Store the retry job
			s.jobs[retryJob.ID] = retryJob

			// Clear the NextRetryAt to prevent duplicate retries
			job.NextRetryAt = time.Time{}

			// Don't hold the mutex during queue operation
			jobCopy := *retryJob
			s.jobsMutex.Unlock()

			fmt.Printf("Queueing retry job %s (attempt %d of %d)\n",
				retryJob.ID, retryJob.RetryCount, retryJob.MaxRetries)
			err := s.QueueJob(&jobCopy)
			if err != nil {
				fmt.Printf("Failed to queue retry job %s: %v\n", retryJob.ID, err)
			}

			s.jobsMutex.Lock()
		}
	}
}
