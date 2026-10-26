package model

import graphql "github.com/graph-gophers/graphql-go"

type Post struct {
	ID              graphql.ID
	AuthorID        graphql.ID
	CommentsEnabled bool
	Title           string
	Body            string
}

type Comment struct {
	ID           graphql.ID
	PostID       graphql.ID
	ParentID     *graphql.ID
	Body         string
	RepliesCount int32
}
