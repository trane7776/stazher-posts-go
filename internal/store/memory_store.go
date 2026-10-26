package store

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	graphql "github.com/graph-gophers/graphql-go"

	"stazher-posts-go/internal/model"
)

type commentBranch struct {
	postID   graphql.ID
	parentID graphql.ID // пустая строка — корневые комментарии
}

type MemoryStore struct {
	mu          sync.RWMutex
	postByID    map[graphql.ID]model.Post
	postIDs     []graphql.ID
	commentByID map[graphql.ID]model.Comment
	//id поста -> срез id комментариев
	commentIDsByPost   map[graphql.ID][]int64
	commentIDsByParent map[commentBranch][]int64
	nextPostID         int64
	nextCommentID      int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		postByID:           make(map[graphql.ID]model.Post),
		commentByID:        make(map[graphql.ID]model.Comment),
		commentIDsByPost:   make(map[graphql.ID][]int64),
		commentIDsByParent: make(map[commentBranch][]int64),
	}
}

func (r *MemoryStore) Post(ctx context.Context, id graphql.ID) (*model.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	post, ok := r.postByID[id]
	if !ok {
		return nil, fmt.Errorf("пост с id %q не найден", id)
	}
	return &post, nil
}

func (r *MemoryStore) Posts(ctx context.Context, limit, offset int32) ([]model.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit должен быть от 1 до 100")
	}
	if offset < 0 {
		return nil, fmt.Errorf("offset не должен быть отрицательным")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	start := int(offset)
	if start >= len(r.postIDs) {
		return []model.Post{}, nil
	}
	end := min(start+int(limit), len(r.postIDs))
	posts := make([]model.Post, 0, end-start)

	for _, id := range r.postIDs[start:end] {
		posts = append(posts, r.postByID[id])
	}
	return posts, nil
}

func (r *MemoryStore) CreatePost(ctx context.Context, authorID graphql.ID, title, body string) (*model.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("заголовок и текст поста не должны быть пустыми")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextPostID++
	post := model.Post{
		ID:              graphql.ID(strconv.FormatInt(r.nextPostID, 10)),
		AuthorID:        authorID,
		CommentsEnabled: true,
		Title:           title,
		Body:            body,
	}
	r.postByID[post.ID] = post
	r.postIDs = append(r.postIDs, post.ID)
	return &post, nil
}

func (r *MemoryStore) SetCommentsEnabled(ctx context.Context, postID, authorID graphql.ID, enabled bool) (*model.Post, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	post, ok := r.postByID[postID]
	if !ok {
		return nil, fmt.Errorf("пост с id %q не найден", postID)
	}
	if post.AuthorID != authorID {
		return nil, fmt.Errorf("только автор поста может менять настройку комментариев")
	}
	post.CommentsEnabled = enabled
	r.postByID[postID] = post
	return &post, nil
}

func (r *MemoryStore) CreateComment(ctx context.Context, postID graphql.ID, parentID *graphql.ID, body string) (*model.Comment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("текст комментария не должен быть пустым")
	}
	if utf8.RuneCountInString(body) > 2000 {
		return nil, fmt.Errorf("комментарий не должен превышать 2000 символов")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	post, ok := r.postByID[postID]
	if !ok {
		return nil, fmt.Errorf("пост с id %q не найден", postID)
	}
	if !post.CommentsEnabled {
		return nil, fmt.Errorf("комментарии к этому посту отключены")
	}
	if parentID != nil {
		parent, ok := r.commentByID[*parentID]
		if !ok {
			return nil, fmt.Errorf("родительский комментарий с id %q не найден", *parentID)
		}
		if parent.PostID != postID {
			return nil, fmt.Errorf("родительский комментарий относится к другому посту")
		}
	}
	r.nextCommentID++
	comment := model.Comment{
		ID:       graphql.ID(strconv.FormatInt(r.nextCommentID, 10)),
		PostID:   postID,
		ParentID: parentID,
		Body:     body,
	}
	r.commentByID[comment.ID] = *copyComment(comment)
	r.commentIDsByPost[postID] = append(r.commentIDsByPost[postID], r.nextCommentID)
	branch := commentBranch{postID: postID}
	if parentID != nil {
		branch.parentID = *parentID
	}
	r.commentIDsByParent[branch] = append(r.commentIDsByParent[branch], r.nextCommentID)
	return copyComment(comment), nil
}

func (r *MemoryStore) Comments(ctx context.Context, postID graphql.ID, limit int32, after *graphql.ID, filter CommentFilter) ([]model.Comment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.commentIDsByPost[postID]
	if filter.RootsOnly {
		ids = r.commentIDsByParent[commentBranch{postID: postID}]
	} else if filter.ParentID != nil {
		ids = r.commentIDsByParent[commentBranch{
			postID:   postID,
			parentID: graphql.ID(strconv.FormatInt(parentID, 10)),
		}]
	}

	start := sort.Search(len(ids), func(i int) bool {
		return ids[i] > afterID
	})
	end := min(start+int(limit), len(ids))
	comments := make([]model.Comment, 0, end-start)

	for _, id := range ids[start:end] {
		comment := r.commentByID[graphql.ID(strconv.FormatInt(id, 10))]
		comment.RepliesCount = int32(len(r.commentIDsByParent[commentBranch{
			postID:   postID,
			parentID: comment.ID,
		}]))
		comments = append(comments, *copyComment(comment))
	}
	return comments, nil
}

func copyComment(comment model.Comment) *model.Comment {
	if comment.ParentID != nil {
		id := *comment.ParentID // разыменовывем
		comment.ParentID = &id  // указываем адрес
	}
	return &comment
}
