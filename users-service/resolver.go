package main

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolver is the root GraphQL resolver: the dependency-injection root for
// every sub-resolver. Kept intentionally thin — business logic lives in the
// resolver methods / helpers below, DB access goes through pgxpool directly
// (no repository layer indirection needed at this scale).
type Resolver struct {
	DB *pgxpool.Pool
}

func (r *Resolver) Query() QueryResolver       { return &queryResolver{r} }
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }
func (r *Resolver) Entity() EntityResolver     { return &entityResolver{r} }

type queryResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type entityResolver struct{ *Resolver }

// --- Queries ---

func (q *queryResolver) User(ctx context.Context, id string) (*User, error) {
	return findUserByID(ctx, q.DB, id)
}

func (q *queryResolver) Users(ctx context.Context, limit *int, offset *int) ([]*User, error) {
	l, o := 20, 0
	if limit != nil {
		l = *limit
	}
	if offset != nil {
		o = *offset
	}

	rows, err := q.DB.Query(ctx, `
		SELECT id, username, email, full_name, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, l, o)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// --- Mutations ---

func (m *mutationResolver) CreateUser(ctx context.Context, input NewUserInput) (*User, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	_, err := m.DB.Exec(ctx, `
		INSERT INTO users (id, username, email, full_name, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)`,
		id, input.Username, input.Email, input.FullName, now)
	if err != nil {
		return nil, err
	}

	return &User{
		ID:        id,
		Username:  input.Username,
		Email:     input.Email,
		FullName:  input.FullName,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (m *mutationResolver) UpdateUser(ctx context.Context, id string, input UpdateUserInput) (*User, error) {
	existing, err := findUserByID(ctx, m.DB, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("user not found")
	}

	if input.Username != nil {
		existing.Username = *input.Username
	}
	if input.Email != nil {
		existing.Email = *input.Email
	}
	if input.FullName != nil {
		existing.FullName = *input.FullName
	}
	existing.UpdatedAt = time.Now().UTC()

	_, err = m.DB.Exec(ctx, `
		UPDATE users SET username = $1, email = $2, full_name = $3, updated_at = $4
		WHERE id = $5`,
		existing.Username, existing.Email, existing.FullName, existing.UpdatedAt, existing.ID)
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (m *mutationResolver) DeleteUser(ctx context.Context, id string) (bool, error) {
	tag, err := m.DB.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// --- Federation entity resolver ---
// Invoked by the router whenever another subgraph (todos-service) needs to
// resolve a User representation it only knows the `id` of.

func (e *entityResolver) FindUserByID(ctx context.Context, id string) (*User, error) {
	return findUserByID(ctx, e.DB, id)
}

// --- Helpers ---

func findUserByID(ctx context.Context, db *pgxpool.Pool, id string) (*User, error) {
	row := db.QueryRow(ctx, `
		SELECT id, username, email, full_name, created_at, updated_at
		FROM users WHERE id = $1`, id)

	u, err := scanUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.FullName, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}
