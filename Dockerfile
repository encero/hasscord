# Build the application
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./ 
RUN go mod download

COPY . .

# Build with build time information
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags "-X main.BuildTime=$(date -u +'%Y-%m-%d %H:%M:%S UTC') \
              -X main.BuildCommit=$(git rev-parse --short HEAD 2>/dev/null || echo 'unknown') \
              -X main.BuildDate=$(date -u +'%Y-%m-%d')" \
    -o hasscord .

# Create the final image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/hasscord .

CMD ["./hasscord"]
