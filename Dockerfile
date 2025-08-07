# Build the application
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum *.go bot/ commands/ config/ hass/ /app/
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o hasscord .

# Create the final image
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/hasscord .

CMD ["./hasscord"]
