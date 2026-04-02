FROM golang:1.24-alpine AS builder
RUN go build -o /app .
