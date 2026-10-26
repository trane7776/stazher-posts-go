package graph

import (
	"context"
	"fmt"

	graphql "github.com/graph-gophers/graphql-go"

	"stazher-posts-go/internal/model"
	"stazher-posts-go/internal/store"
)

type resolver struct{ store store.Store }

func newResolver(storage store.Store) *resolver { return &resolver{store: storage} }

func (r *resolver) Hello() string {
	return "Hello World!"
}

func (r *resolver) Post(ctx context.Context, args struct{ ID graphql.ID }) (*model.Post, error) {
	return r.store.Post(ctx, args.ID)
}

func (r *resolver) Posts(ctx context.Context, args struct{ Limit, Offset int32 }) ([]model.Post, error) {
	return r.store.Posts(ctx, args.Limit, args.Offset)
}

func (r *resolver) CreatePost(ctx context.Context, args struct{ Title, Body string }) (*model.Post, error) {
	userID, _ := ctx.Value(userIDKey{}).(string)
	if userID == "" {
		return nil, fmt.Errorf("для создания поста передайте заголовок X-User-ID")
	}
	return r.store.CreatePost(ctx, graphql.ID(userID), args.Title, args.Body)
}

func (r *resolver) SetCommentsEnabled(ctx context.Context, args struct {
	PostID  graphql.ID
	Enabled bool
}) (*model.Post, error) {
	userID, _ := ctx.Value(userIDKey{}).(string)
	if userID == "" {
		return nil, fmt.Errorf("для изменения настройки передайте заголовок X-User-ID")
	}
	return r.store.SetCommentsEnabled(ctx, args.PostID, graphql.ID(userID), args.Enabled)
}

func (r *resolver) CreateComment(ctx context.Context, args struct {
	PostID   graphql.ID
	ParentID *graphql.ID
	Body     string
}) (*model.Comment, error) {
	return r.store.CreateComment(ctx, args.PostID, args.ParentID, args.Body)
}

func (r *resolver) Comments(ctx context.Context, args struct {
	PostID    graphql.ID
	ParentID  *graphql.ID
	RootsOnly bool
	Limit     int32
	After     *graphql.ID
}) ([]model.Comment, error) {
	return r.store.Comments(ctx, args.PostID, args.Limit, args.After, store.CommentFilter{
		ParentID:  args.ParentID,
		RootsOnly: args.RootsOnly,
	})
}
