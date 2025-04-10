package main

import (
	"bytes"
	"context"
	"fmt"

	"os/exec"
	"strings"
	"time"
)

type Worker struct {
	jobService *JobService
}

func NewWorker(jobService *JobService) *Worker {
	return &Worker{
		jobService: jobService,
	}
}

func (w *Worker) Start(ctx context.Context) {
	fmt.Println("Worker started, waiting for jobs...")

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Worker shutting down...")
			return
		default:
			job, err := w.jobService.DequeueJob(ctx)
			if err != nil {
				if err == context.Canceled {
					return
				}
				fmt.Printf("Error dequeuing job: %v\n", err)
				time.Sleep(1 * time.Second)
				continue
			}

			fmt.Printf("Processing job %s: %s\n", job.ID, job.Command)
			w.processJob(job)
		}
	}
}

func (w *Worker) processJob(job *Job) {

	err := w.jobService.UpdateJobStatus(job.ID, JobStatusRunning, "", "")
	if err != nil {
		fmt.Printf("Failed to update job status: %v\n", err)
	}

	cmd := exec.Command("sh", "-c", job.Command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

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

	err = w.jobService.UpdateJobStatus(job.ID, status, output, errMsg)
	if err != nil {
		fmt.Printf("Failed to update job status: %v\n", err)
	}

	fmt.Printf("Job %s completed with status: %s\n", job.ID, status)
}
