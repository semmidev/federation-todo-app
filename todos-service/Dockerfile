# --- build stage ---
FROM golang:1.26.4-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY . .
RUN go mod tidy
RUN go generate ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/todos-service .

# --- final stage ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/todos-service /todos-service
USER nonroot:nonroot
EXPOSE 4002
ENTRYPOINT ["/todos-service"]
