# --- build stage ---
FROM golang:1.26.4-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY . .
# go.sum isn't vendored in this template; resolve + generate + build in one shot.
RUN go mod tidy
RUN go generate ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/users-service .

# --- final stage ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/users-service /users-service
USER nonroot:nonroot
EXPOSE 4001
ENTRYPOINT ["/users-service"]
