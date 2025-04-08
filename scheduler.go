package main

import (
	"fmt"

	"github.com/robfig/cron/v3"
)

// Scheduler handles periodic cron jobs
type Scheduler struct {
	cron       *cron.Cron
	jobService *JobService
}

// NewScheduler creates a new scheduler
func NewScheduler(jobService *JobService) *Scheduler {
	// Create a new cron scheduler with second precision
	cronScheduler := cron.New(cron.WithSeconds())
	
	return &Scheduler{
		cron:       cronScheduler,
		jobService: jobService,
	}
}

// Start starts the scheduler and loads any existing cron jobs
func (s *Scheduler) Start() {
	// Schedule any existing cron jobs
	jobs := s.jobService.GetAllJobs()
	for _, job := range jobs {
		if job.Type == JobTypeCron {
			s.ScheduleJob(job)
		}
	}
	
	// Start the cron scheduler
	s.cron.Start()
	fmt.Println("Scheduler started")
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
	fmt.Println("Scheduler stopped")
}

// ScheduleJob schedules a job to run according to its cron schedule
func (s *Scheduler) ScheduleJob(job *Job) error {
	if job.Type != JobTypeCron || job.Schedule == "" {
		return fmt.Errorf("job is not a cron job or missing schedule")
	}
	
	_, err := s.cron.AddFunc(job.Schedule, func() {
		fmt.Printf("Executing scheduled job %s: %s\n", job.ID, job.Command)
		
		// Create a clone of the job for execution
		jobClone := &Job{
			ID:        job.ID + "-" + fmt.Sprintf("%d", time.Now().Unix()),
			Command:   job.Command,
			Type:      JobTypeOnce,  // Execute as a one-time job
			Status:    JobStatusPending,
			CreatedAt: time.Now(),
		}
		
		// Store the clone
		s.jobService.jobsMutex.Lock()
		s.jobService.jobs[jobClone.ID] = jobClone
		s.jobService.jobsMutex.Unlock()
		
		// Queue the job
		err := s.jobService.QueueJob(jobClone)
		if err != nil {
			fmt.Printf("Failed to queue scheduled job: %v\n", err)
		}
	})
	
	return err
}