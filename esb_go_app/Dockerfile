FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# Pass the version number as a build argument: --build-arg VERSION=v1.0.0
ARG VERSION="2.0.0"
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags "-s -w -X main.version=${VERSION}" -o /app/main .


FROM alpine:latest

# Install tzdata package for timezone support
RUN apk add --no-cache tzdata

WORKDIR /app

COPY --from=builder /app/main .
COPY --from=builder /app/templates /app/templates/
COPY config.json .

EXPOSE 8080

CMD ["/app/main"]
