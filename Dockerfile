FROM golang:1.27-alpine AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /api ./cmd/api

FROM alpine:3.22
RUN adduser -D -H appuser
USER appuser
COPY --from=build /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]

