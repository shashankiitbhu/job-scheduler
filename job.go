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
        if err := s.QueueJob(job); err != nil {
            // Rollback memory map if queue fails
            s.jobsMutex.Lock()
            delete(s.jobs, job.ID)
            s.jobsMutex.Unlock()
            return nil, err
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