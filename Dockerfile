# Build the application
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./ 
RUN go mod download

COPY . .

# Build with build time information
RUN BUILD_TIME="$(date -u +'%Y-%m-%d %H:%M:%S UTC')" \
  BUILD_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)" \
  BUILD_DATE="$(date -u +'%Y-%m-%d')" && \
  CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X main.BuildTime=${BUILD_TIME} -X main.BuildCommit=${BUILD_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
    -o hasscord .

# Create the final image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/hasscord .

CMD ["./hasscord"]
