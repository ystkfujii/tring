FROM golang:1.24.1 AS builder
RUN go build -o /app .

FROM debian:12.8
COPY --from=builder /app /app
