# First stage: Build the Go binary
FROM golang:1.24-alpine AS build
WORKDIR /app
COPY . .
RUN go build -o webhook

# Second stage: Minimal runtime image
FROM alpine:latest
WORKDIR /root/
COPY --from=build /app/webhook .
CMD ["./webhook"]
