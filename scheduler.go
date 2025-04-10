package main

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron       *cron.Cron
	jobService *JobService
}

func NewScheduler(jobService *JobService) *Scheduler {
	cronScheduler := cron.New(cron.WithSeconds())

	return &Scheduler{
		cron:       cronScheduler,
		jobService: jobService,
	}
}

func (s *Scheduler) Start() {
	jobs := s.jobService.GetAllJobs()
	for _, job := range jobs {
		if job.Type == JobTypeCron {
			s.ScheduleJob(job)
		}
	}

	// Add a periodic job to check for delayed jobs
	s.cron.AddFunc("*/10 * * * * *", func() {
		s.jobService.CheckDelayedJobs()
	})

	s.cron.Start()
	fmt.Println("Scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
	fmt.Println("Scheduler stopped")
}

func (s *Scheduler) ScheduleJob(job *Job) error {
	if job.Type != JobTypeCron || job.Schedule == "" {
		return fmt.Errorf("job is not a cron job or missing schedule")
	}

	_, err := s.cron.AddFunc(job.Schedule, func() {
		fmt.Printf("Executing scheduled job %s: %s\n", job.ID, job.Command)

		jobClone := &Job{
			ID:        job.ID + "-" + fmt.Sprintf("%d", time.Now().Unix()),
			Command:   job.Command,
			Type:      JobTypeOnce, // Execute as a one-time job
			Status:    JobStatusPending,
			CreatedAt: time.Now(),
		}

		s.jobService.jobsMutex.Lock()
		s.jobService.jobs[jobClone.ID] = jobClone
		s.jobService.jobsMutex.Unlock()

		err := s.jobService.QueueJob(jobClone)
		if err != nil {
			fmt.Printf("Failed to queue scheduled job: %v\n", err)
		}
	})

	return err
}
