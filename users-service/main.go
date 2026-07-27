//go:generate go run github.com/99designs/gqlgen generate
package main

import (
	"context"
	_ "embed"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

const defaultPort = "4001"

func main() {
	// Self-healthcheck mode: used by Docker's HEALTHCHECK / docker-compose
	// healthcheck against a distroless image with no shell/curl available.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	port := getEnv("PORT", defaultPort)
	dsn := getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/users_db?sslmode=disable")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		logger.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("ping database", "error", err)
		os.Exit(1)
	}

	if _, err := pool.Exec(context.Background(), schemaSQL); err != nil {
		logger.Error("run migrations", "error", err)
		os.Exit(1)
	}

	resolver := &Resolver{DB: pool}
	srv := handler.NewDefaultServer(NewExecutableSchema(Config{Resolvers: resolver}))

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Handle("/", playground.Handler("Users Subgraph", "/query"))
	r.Handle("/query", srv)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	logger.Info("users subgraph listening", "port", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func runHealthcheck() int {
	port := getEnv("PORT", defaultPort)
	resp, err := http.Get("http://localhost:" + port + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
