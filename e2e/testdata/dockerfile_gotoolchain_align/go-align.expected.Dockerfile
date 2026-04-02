FROM golang:1.22.2 AS builder
RUN go build -o /app .

FROM debian:12.10
COPY --from=builder /app /app
