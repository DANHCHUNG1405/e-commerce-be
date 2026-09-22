FROM golang:1.27-alpine AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG APP_BINARY=api
RUN CGO_ENABLED=0 GOOS=linux go build -o /app-bin ./cmd/${APP_BINARY}

FROM alpine:3.22
RUN adduser -D -H appuser
USER appuser
COPY --from=build /app-bin /app-bin
EXPOSE 8080 8082 8083 8084 9091 9092
ENTRYPOINT ["/app-bin"]
