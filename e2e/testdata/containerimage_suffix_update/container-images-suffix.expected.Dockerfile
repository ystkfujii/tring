FROM golang:1.25-alpine AS builder
RUN go build -o /app .
