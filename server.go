package main

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Server struct {
	jobService *JobService
	port       int
}

func NewServer(jobService *JobService, port int) *Server {
	return &Server{
		jobService: jobService,
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
	}
	
	return router.Run(fmt.Sprintf(":%d", s.port))
}

func (s *Server) createJob(c *gin.Context) {
	var request struct {
		Command  string `json:"command" binding:"required"`
		Type     string `json:"type" binding:"required"`
		Schedule string `json:"schedule"`
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
	
	job, err := s.jobService.CreateJob(request.Command, jobType, request.Schedule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusCreated, job)
}

func (s *Server) listJobs(c *gin.Context) {
	jobs := s.jobService.GetAllJobs()
	c.JSON(http.StatusOK, jobs)
}

func (s *Server) getJob(c *gin.Context) {
	id := c.Param("id")
	job, err := s.jobService.GetJob(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	
	c.JSON(http.StatusOK, job)
}