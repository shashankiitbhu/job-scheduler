# Go-Based Distributed Job Scheduler

A lightweight, modular job scheduler built with Go and Redis. This system allows you to submit one-time or cron-scheduled jobs through a REST API and execute them in a distributed environment.

## Features

- **REST API** for job management
- **Redis-based job queue** for reliable job distribution
- **Cron scheduling** for periodic tasks
- **Web UI** for easy job submission and monitoring
- **Distributed architecture** with separate API and worker components
- **Docker support** for easy deployment

## Architecture

The system consists of three main components:

1. **API Server**: Handles job submissions and provides job status information
2. **Worker**: Processes jobs from the Redis queue
3. **Redis**: Acts as the job queue and optional metadata store

## Prerequisites

- Go 1.16+ (for local development)
- Docker and Docker Compose (for containerized deployment)
- Redis (included in Docker setup)

## Quick Start with Docker

The easiest way to get started is using Docker Compose:

1. Clone this repository:
   ```bash
   git clone https://github.com/yourusername/job-scheduler.git
   cd job-scheduler
   ```

2. Start the services:
   ```bash
   docker-compose up --build
   ```

3. Access the web UI:
   ```
   http://localhost:8080
   ```

## Manual Setup

If you prefer to run the components manually:

1. Start Redis:
   ```bash
   docker run -d -p 6379:6379 --name redis redis:alpine
   ```

2. Install Go dependencies:
   ```bash
   go mod tidy
   ```

3. Run the API server:
   ```bash
   go run *.go --port 8080
   ```

4. In a separate terminal, run the worker:
   ```bash
   go run *.go --worker
   ```

5. Access the web UI:
   ```
   http://localhost:8080
   ```

## API Endpoints

### Submit a new job

```
POST /api/jobs
```

**Request Body**:

```json
{
  "command": "echo 'Hello, World!'",
  "type": "once",
  "schedule": ""
}
```

For a cron job:

```json
{
  "command": "date >> /tmp/date.log",
  "type": "cron",
  "schedule": "*/10 * * * * *"
}
```

**Response**:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "command": "echo 'Hello, World!'",
  "type": "once",
  "status": "pending",
  "created_at": "2025-04-09T12:34:56Z"
}
```

### List all jobs

```
GET /api/jobs
```

### Get a specific job

```
GET /api/jobs/:id
```

## Cron Schedule Format

The scheduler uses a cron expression with seconds precision. The format is:

```
second minute hour day_of_month month day_of_week
```

Examples:
- Every minute: `0 * * * * *`
- Every 30 seconds: `*/30 * * * * *`
- Every day at midnight: `0 0 0 * * *`

## Configuration Options

The following command-line flags are available:

- `--port`: HTTP server port (default: 8080)
- `--worker`: Run as a worker instead of API server
- `--redis-addr`: Redis address (default: localhost:6379)
- `--redis-db`: Redis database number (default: 0)
- `--redis-pass`: Redis password (if required)

## Project Structure

```
.
├── Dockerfile              # Docker build instructions
├── docker-compose.yml      # Docker Compose configuration
├── go.mod                  # Go module definition
├── go.sum                  # Go module checksums
├── main.go                 # Application entry point
├── job.go                  # Job definitions and service
├── worker.go               # Worker for processing jobs
├── server.go               # HTTP server and API
├── scheduler.go            # Cron scheduler
└── static/                 # Frontend files
    └── index.html          # Web UI
```

## Use Cases

- **Scheduled Backups**: Run backup scripts at specified intervals
- **Data Processing**: Process data files in the background
- **Periodic Reporting**: Generate reports on a schedule
- **Resource-Intensive Tasks**: Offload heavy operations to worker nodes
- **Maintenance Operations**: Run cleanup tasks during off-hours

## Troubleshooting

### Common Issues

1. **Cannot connect to Redis**:
   - Check if Redis is running: `docker ps`
   - Verify Redis address and credentials

2. **Jobs are not being processed**:
   - Ensure the worker is running
   - Check Redis connection
   - Verify jobs are being pushed to the queue

3. **API server not starting**:
   - Check for port conflicts
   - Ensure Go dependencies are installed

### Logging

- The API server and worker output logs to stdout
- Check Docker logs: `docker-compose logs -f`

## Extending the System

Here are some ways to extend the functionality:

1. **Authentication**: Add JWT or API key authentication
2. **Job Priorities**: Implement priority queues
3. **Retries**: Add automatic retry for failed jobs
4. **Metrics**: Integrate Prometheus for monitoring
5. **Persistence**: Add database storage for job history

## License

MIT License - See LICENSE file for details.