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
	ID        string    `json:"id"`
	Command   string    `json:"command"`
	Type      JobType   `json:"type"`
	Schedule  string    `json:"schedule,omitempty"` 
	Status    JobStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	StartedAt time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Output    string    `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type JobService struct {
	redis    *redis.Client
	jobs     map[string]*Job 
	jobsMutex sync.RWMutex   // Mutex for concurrent access to the jobs map
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

func (s *JobService) CreateJob(cmd string, jobType JobType, schedule string) (*Job, error) {
	if cmd == "" {
		return nil, errors.New("command cannot be empty")
	}

	if jobType == JobTypeCron && schedule == "" {
		return nil, errors.New("cron jobs require a schedule")
	}

	job := &Job{
		ID:        uuid.New().String(),
		Command:   cmd,
		Type:      jobType,
		Schedule:  schedule,
		Status:    JobStatusPending,
		CreatedAt: time.Now(),
	}

	s.jobsMutex.Lock()
	s.jobs[job.ID] = job
	s.jobsMutex.Unlock()

	
	if jobType == JobTypeOnce {
		err := s.QueueJob(job)
		if err != nil {
			return nil, fmt.Errorf("failed to queue job: %w", err)
		}
	}

	return job, nil
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

	job.Status = status

	if status == JobStatusRunning && job.StartedAt.IsZero() {
		job.StartedAt = time.Now()
	}

	if status == JobStatusCompleted || status == JobStatusFailed {
		job.FinishedAt = time.Now()
		job.Output = output
		job.Error = errMsg
	}

	return nil
}

func (s *JobService) DequeueJob(ctx context.Context) (*Job, error) {
	// Use BLPOP to wait for a job
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