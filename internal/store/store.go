package store

import (
	"context"
	"fmt"
	"strconv"

	graphql "github.com/graph-gophers/graphql-go"

	"stazher-posts-go/internal/model"
)

type CommentFilter struct {
	ParentID  *graphql.ID
	RootsOnly bool
}

type Store interface {
	Post(ctx context.Context, id graphql.ID) (*model.Post, error)
	Posts(ctx context.Context, limit, offset int32) ([]model.Post, error)
	CreatePost(ctx context.Context, authorID graphql.ID, title, body string) (*model.Post, error)
	SetCommentsEnabled(ctx context.Context, postID, authorID graphql.ID, enabled bool) (*model.Post, error)
	CreateComment(ctx context.Context, postID graphql.ID, parentID *graphql.ID, body string) (*model.Comment, error)
	Comments(ctx context.Context, postID graphql.ID, limit int32, after *graphql.ID, filter CommentFilter) ([]model.Comment, error)
}

func parseCommentParent(filter CommentFilter) (int64, error) {
	if filter.RootsOnly && filter.ParentID != nil {
		return 0, fmt.Errorf("нельзя одновременно передавать rootsOnly и parentID")
	}
	if filter.ParentID == nil {
		return 0, nil
	}
	id, err := strconv.ParseInt(string(*filter.ParentID), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("parentID должен быть положительным целым ID комментария")
	}
	return id, nil
}

// отсутствие курсора означает начало списка
func parseCommentCursor(after *graphql.ID) (int64, error) {
	if after == nil {
		return 0, nil
	}
	id, err := strconv.ParseInt(string(*after), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("after должен быть положительным целым ID комментария")
	}
	return id, nil
}
