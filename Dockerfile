FROM golang:1.21-alpine AS builder

WORKDIR /app


COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /go/bin/scheduler

FROM alpine:latest

RUN apk --no-cache add bash

COPY --from=builder /go/bin/scheduler /usr/local/bin/scheduler

WORKDIR /app
COPY static/ /app/static/

EXPOSE 8080

ENTRYPOINT ["scheduler"]