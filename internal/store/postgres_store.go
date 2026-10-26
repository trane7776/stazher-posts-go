package store

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"stazher-posts-go/internal/model"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func (s *PostgresStore) Comments(ctx context.Context, postID graphql.ID, limit int32, after *graphql.ID, filter CommentFilter) ([]model.Comment, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit должен быть от 1 до 100")
	}
	afterID, err := parseCommentCursor(after)
	if err != nil {
		return nil, err
	}
	parentID, err := parseCommentParent(filter)
	if err != nil {
		return nil, err
	}
	numericPostID, err := strconv.ParseInt(string(postID), 10, 64)
	if err != nil || numericPostID <= 0 {
		return nil, fmt.Errorf("ID поста должен быть положительным целым числом")
	}

	query := `
		SELECT id, post_id, parent_id, body, replies_count
		FROM comments
		WHERE post_id = $1 AND id > $2
	`
	args := []any{numericPostID, afterID, limit}
	if filter.RootsOnly {
		query += ` AND parent_id IS NULL`
	} else if filter.ParentID != nil {
		query += ` AND parent_id = $4`
		args = append(args, parentID)
	}
	query += ` ORDER BY id LIMIT $3`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("запрос комментариев: %w", err)
	}
	defer rows.Close()

	comments := []model.Comment{}
	for rows.Next() {
		var comment model.Comment
		var dbID, dbPostID int64
		var dbParentID *int64
		if err := rows.Scan(&dbID, &dbPostID, &dbParentID, &comment.Body, &comment.RepliesCount); err != nil {
			return nil, fmt.Errorf("чтение строки комментария: %w", err)
		}
		comment.ID = graphql.ID(strconv.FormatInt(dbID, 10))
		comment.PostID = graphql.ID(strconv.FormatInt(dbPostID, 10))
		if dbParentID != nil {
			id := graphql.ID(strconv.FormatInt(*dbParentID, 10))
			comment.ParentID = &id
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение комментариев: %w", err)
	}
	return comments, nil
}

func (s *PostgresStore) CreateComment(ctx context.Context, postID graphql.ID, parentID *graphql.ID, body string) (*model.Comment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("текст комментария не должен быть пустым")
	}
	if utf8.RuneCountInString(body) > 2000 {
		return nil, fmt.Errorf("комментарий не должен превышать 2000 символов")
	}
	numericPostID, err := strconv.ParseInt(string(postID), 10, 64)
	if err != nil || numericPostID <= 0 {
		return nil, fmt.Errorf("ID поста должен быть положительным целым числом")
	}
	var numericParentID *int64
	if parentID != nil {
		id, err := strconv.ParseInt(string(*parentID), 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("ID родителя должен быть положительным целым числом")
		}
		numericParentID = &id
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("начало транзакции: %w", err)
	}
	defer tx.Rollback(context.Background())

	var enabled bool
	err = tx.QueryRow(ctx, `SELECT comments_enabled FROM posts WHERE id = $1 FOR UPDATE`, numericPostID).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("пост с id %q не найден", postID)
	}
	if err != nil {
		return nil, fmt.Errorf("проверка настройки комментариев: %w", err)
	}
	if !enabled {
		return nil, fmt.Errorf("комментарии к этому посту отключены")
	}

	if numericParentID != nil {
		var parentPostID int64
		err = tx.QueryRow(ctx, `SELECT post_id FROM comments WHERE id = $1`, *numericParentID).Scan(&parentPostID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("родительский комментарий с id %q не найден", *parentID)
		}
		if err != nil {
			return nil, fmt.Errorf("проверка родительского комментария: %w", err)
		}
		if parentPostID != numericPostID {
			return nil, fmt.Errorf("родительский комментарий относится к другому посту")
		}
	}

	var comment model.Comment
	var savedID, savedPostID int64
	var savedParentID *int64
	err = tx.QueryRow(ctx, `
		INSERT INTO comments (post_id, parent_id, body)
		VALUES ($1, $2, $3)
		RETURNING id, post_id, parent_id, body, replies_count
	`, numericPostID, numericParentID, body).Scan(&savedID, &savedPostID, &savedParentID, &comment.Body, &comment.RepliesCount)
	if err != nil {
		return nil, fmt.Errorf("сохранение комментария: %w", err)
	}
	if numericParentID != nil {
		updated, err := tx.Exec(ctx, `
			UPDATE comments
			SET replies_count = replies_count + 1
			WHERE id = $1 AND post_id = $2
		`, *numericParentID, numericPostID)
		if err != nil {
			return nil, fmt.Errorf("обновление счётчика ответов: %w", err)
		}
		if updated.RowsAffected() != 1 {
			return nil, fmt.Errorf("родительский комментарий с id %q не найден", *parentID)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("подтверждение транзакции: %w", err)
	}

	comment.ID = graphql.ID(strconv.FormatInt(savedID, 10))
	comment.PostID = graphql.ID(strconv.FormatInt(savedPostID, 10))
	if savedParentID != nil {
		id := graphql.ID(strconv.FormatInt(*savedParentID, 10))
		comment.ParentID = &id
	}
	return &comment, nil
}

func (s *PostgresStore) SetCommentsEnabled(ctx context.Context, postID, authorID graphql.ID, enabled bool) (*model.Post, error) {
	if strings.TrimSpace(string(authorID)) == "" {
		return nil, fmt.Errorf("укажите автора поста")
	}
	numericID, err := strconv.ParseInt(string(postID), 10, 64)
	if err != nil || numericID <= 0 {
		return nil, fmt.Errorf("ID поста должен быть положительным целым числом")
	}

	var post model.Post
	var dbID int64
	var savedAuthorID string
	err = s.pool.QueryRow(ctx, `
		UPDATE posts
		SET comments_enabled = $3
		WHERE id = $1 AND author_id = $2
		RETURNING id, author_id, title, body, comments_enabled
	`, numericID, string(authorID), enabled).Scan(&dbID, &savedAuthorID, &post.Title, &post.Body, &post.CommentsEnabled)
	if errors.Is(err, pgx.ErrNoRows) {

		if _, err := s.Post(ctx, postID); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("только автор поста может менять настройку комментариев")
	}
	if err != nil {
		return nil, fmt.Errorf("изменение настройки комментариев в PostgreSQL: %w", err)
	}

	post.ID = graphql.ID(strconv.FormatInt(dbID, 10))
	post.AuthorID = graphql.ID(savedAuthorID)
	return &post, nil
}

func (s *PostgresStore) CreatePost(ctx context.Context, authorID graphql.ID, title, body string) (*model.Post, error) {
	if strings.TrimSpace(string(authorID)) == "" {
		return nil, fmt.Errorf("укажите автора поста")
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("заголовок и текст поста не должны быть пустыми")
	}

	var post model.Post
	var dbID int64
	var savedAuthorID string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO posts (author_id, title, body)
		VALUES ($1, $2, $3)
		RETURNING id, author_id, title, body, comments_enabled
	`, string(authorID), title, body).Scan(&dbID, &savedAuthorID, &post.Title, &post.Body, &post.CommentsEnabled)
	if err != nil {
		return nil, fmt.Errorf("создание поста в PostgreSQL: %w", err)
	}

	post.ID = graphql.ID(strconv.FormatInt(dbID, 10))
	post.AuthorID = graphql.ID(savedAuthorID)
	return &post, nil
}

func (s *PostgresStore) Post(ctx context.Context, id graphql.ID) (*model.Post, error) {
	numericID, err := strconv.ParseInt(string(id), 10, 64)
	if err != nil || numericID <= 0 {
		return nil, fmt.Errorf("ID поста должен быть положительным целым числом")
	}

	var post model.Post
	var dbID int64
	var authorID string
	err = s.pool.QueryRow(ctx, `
		SELECT id, author_id, title, body, comments_enabled
		FROM posts
		WHERE id = $1
	`, numericID).Scan(&dbID, &authorID, &post.Title, &post.Body, &post.CommentsEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("пост с id %q не найден", id)
	}
	if err != nil {
		return nil, fmt.Errorf("чтение поста из PostgreSQL: %w", err)
	}

	post.ID = graphql.ID(strconv.FormatInt(dbID, 10))
	post.AuthorID = graphql.ID(authorID)
	return &post, nil
}

func (s *PostgresStore) Posts(ctx context.Context, limit, offset int32) ([]model.Post, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit должен быть от 1 до 100")
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset не должен быть отрицательным")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, author_id, title, body, comments_enabled
		FROM posts
		ORDER BY id
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("запрос списка постов: %w", err)
	}
	defer rows.Close()

	posts := []model.Post{}
	for rows.Next() {
		var post model.Post
		var dbID int64
		var authorID string
		if err := rows.Scan(&dbID, &authorID, &post.Title, &post.Body, &post.CommentsEnabled); err != nil {
			return nil, fmt.Errorf("чтение строки поста: %w", err)
		}
		post.ID = graphql.ID(strconv.FormatInt(dbID, 10))
		post.AuthorID = graphql.ID(authorID)
		posts = append(posts, post)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("чтение списка постов: %w", err)
	}
	return posts, nil
}
