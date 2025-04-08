package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Worker processes jobs from the queue
type Worker struct {
	jobService *JobService
}

// NewWorker creates a new worker
func NewWorker(jobService *JobService) *Worker {
	return &Worker{
		jobService: jobService,
	}
}

// Start begins processing jobs from the queue
func (w *Worker) Start(ctx context.Context) {
	fmt.Println("Worker started, waiting for jobs...")
	
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Worker shutting down...")
			return
		default:
			// Try to get a job from the queue
			job, err := w.jobService.DequeueJob(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				fmt.Printf("Error dequeuing job: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}
			
			// Process the job
			fmt.Printf("Processing job %s: %s\n", job.ID, job.Command)
			w.processJob(job)
		}
	}
}

// processJob executes the command associated with a job
func (w *Worker) processJob(job *Job) {
	// Update job status to running
	err := w.jobService.UpdateJobStatus(job.ID, JobStatusRunning, "", "")
	if err != nil {
		fmt.Printf("Failed to update job status: %v\n", err)
	}
	
	// Prepare the command
	cmd := exec.Command("sh", "-c", job.Command)
	
	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Execute the command
	err = cmd.Run()
	
	// Process the results
	output := stdout.String()
	errOutput := stderr.String()
	
	status := JobStatusCompleted
	errMsg := ""
	
	if err != nil {
		status = JobStatusFailed
		errMsg = err.Error()
		if errOutput != "" {
			errMsg = strings.TrimSpace(errOutput)
		}
	}
	
	// Update job status with results
	err = w.jobService.UpdateJobStatus(job.ID, status, output, errMsg)
	if err != nil {
		fmt.Printf("Failed to update job status: %v\n", err)
	}
	
	fmt.Printf("Job %s completed with status: %s\n", job.ID, status)
}