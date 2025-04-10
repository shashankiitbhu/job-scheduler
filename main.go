package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

type Config struct {
	RedisAddr  string
	RedisDB    int
	RedisPass  string
	HTTPPort   int
	WorkerMode bool
}

func main() {

	config := parseFlags()

	redisClient := setupRedisClient(config)
	defer redisClient.Close()

	jobService := NewJobService(redisClient)

	if config.WorkerMode {
		fmt.Println("Starting in worker mode...")
		worker := NewWorker(jobService)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			fmt.Println("Shutting down worker...")
			cancel()
		}()

		worker.Start(ctx)
	} else {

		fmt.Println("Starting in API server mode...")
		scheduler := NewScheduler(jobService)
		scheduler.Start()
		server := NewServer(jobService, scheduler, config.HTTPPort)

		defer scheduler.Stop()

		err := server.Start()
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}
}

func parseFlags() *Config {
	config := &Config{}

	flag.StringVar(&config.RedisAddr, "redis-addr", "localhost:6379", "Redis address")
	flag.IntVar(&config.RedisDB, "redis-db", 0, "Redis database")
	flag.StringVar(&config.RedisPass, "redis-pass", "", "Redis password")
	flag.IntVar(&config.HTTPPort, "port", 8080, "HTTP server port")
	flag.BoolVar(&config.WorkerMode, "worker", false, "Run as a worker")

	flag.Parse()

	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		config.RedisAddr = redisAddr
	}

	return config
}
