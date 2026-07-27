package main

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolver is the root GraphQL resolver / dependency-injection root.
type Resolver struct {
	DB *pgxpool.Pool
}

func (r *Resolver) Query() QueryResolver       { return &queryResolver{r} }
func (r *Resolver) Mutation() MutationResolver { return &mutationResolver{r} }
func (r *Resolver) Entity() EntityResolver     { return &entityResolver{r} }
func (r *Resolver) User() UserResolver         { return &userResolver{r} }

type queryResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type entityResolver struct{ *Resolver }
type userResolver struct{ *Resolver }

// --- Queries ---

func (q *queryResolver) Todo(ctx context.Context, id string) (*Todo, error) {
	return findTodoByID(ctx, q.DB, id)
}

func (q *queryResolver) Todos(ctx context.Context, userID *string, limit *int, offset *int) ([]*Todo, error) {
	l, o := 20, 0
	if limit != nil {
		l = *limit
	}
	if offset != nil {
		o = *offset
	}

	if userID != nil {
		rows, err := q.DB.Query(ctx, `
			SELECT id, title, description, completed, user_id, created_at, updated_at
			FROM todos WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3`, *userID, l, o)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanTodos(rows)
	}

	rows, err := q.DB.Query(ctx, `
		SELECT id, title, description, completed, user_id, created_at, updated_at
		FROM todos
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, l, o)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTodos(rows)
}

// --- Mutations ---

func (m *mutationResolver) CreateTodo(ctx context.Context, input NewTodoInput) (*Todo, error) {
	id := uuid.NewString()
	now := time.Now().UTC()

	_, err := m.DB.Exec(ctx, `
		INSERT INTO todos (id, title, description, completed, user_id, created_at, updated_at)
		VALUES ($1, $2, $3, false, $4, $5, $5)`,
		id, input.Title, input.Description, input.UserID, now)
	if err != nil {
		return nil, err
	}

	return &Todo{
		ID:          id,
		Title:       input.Title,
		Description: input.Description,
		Completed:   false,
		UserID:      input.UserID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (m *mutationResolver) UpdateTodo(ctx context.Context, id string, input UpdateTodoInput) (*Todo, error) {
	existing, err := findTodoByID(ctx, m.DB, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("todo not found")
	}

	if input.Title != nil {
		existing.Title = *input.Title
	}
	if input.Description != nil {
		existing.Description = input.Description
	}
	if input.Completed != nil {
		existing.Completed = *input.Completed
	}
	existing.UpdatedAt = time.Now().UTC()

	_, err = m.DB.Exec(ctx, `
		UPDATE todos SET title = $1, description = $2, completed = $3, updated_at = $4
		WHERE id = $5`,
		existing.Title, existing.Description, existing.Completed, existing.UpdatedAt, existing.ID)
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (m *mutationResolver) DeleteTodo(ctx context.Context, id string) (bool, error) {
	tag, err := m.DB.Exec(ctx, `DELETE FROM todos WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (m *mutationResolver) ToggleTodo(ctx context.Context, id string) (*Todo, error) {
	existing, err := findTodoByID(ctx, m.DB, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("todo not found")
	}

	existing.Completed = !existing.Completed
	existing.UpdatedAt = time.Now().UTC()

	_, err = m.DB.Exec(ctx, `
		UPDATE todos SET completed = $1, updated_at = $2 WHERE id = $3`,
		existing.Completed, existing.UpdatedAt, existing.ID)
	if err != nil {
		return nil, err
	}
	return existing, nil
}

// --- User field resolver (Federation entity extension) ---
// Resolves the `todos` field this subgraph contributes to the `User` entity
// owned by users-service. `obj` here is only a *representation* — it carries
// the `id` key field forwarded by the router, nothing else.

func (u *userResolver) Todos(ctx context.Context, obj *User) ([]*Todo, error) {
	rows, err := u.DB.Query(ctx, `
		SELECT id, title, description, completed, user_id, created_at, updated_at
		FROM todos WHERE user_id = $1
		ORDER BY created_at DESC`, obj.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTodos(rows)
}

// --- Federation entity resolvers ---

func (e *entityResolver) FindTodoByID(ctx context.Context, id string) (*Todo, error) {
	return findTodoByID(ctx, e.DB, id)
}

// FindUserByID reconstructs a minimal User representation for a foreign
// entity: this subgraph never owns User data, it only needs the id to
// resolve the `todos` field above.
func (e *entityResolver) FindUserByID(ctx context.Context, id string) (*User, error) {
	return &User{ID: id}, nil
}

// --- Helpers ---

func findTodoByID(ctx context.Context, db *pgxpool.Pool, id string) (*Todo, error) {
	row := db.QueryRow(ctx, `
		SELECT id, title, description, completed, user_id, created_at, updated_at
		FROM todos WHERE id = $1`, id)

	t, err := scanTodo(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTodo(row rowScanner) (*Todo, error) {
	var t Todo
	if err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Completed, &t.UserID, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTodos(rows pgx.Rows) ([]*Todo, error) {
	var todos []*Todo
	for rows.Next() {
		t, err := scanTodo(rows)
		if err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}
