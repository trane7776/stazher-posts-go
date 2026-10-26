package store

import (
	"strings"
	"sync"
	"testing"

	graphql "github.com/graph-gophers/graphql-go"

	"stazher-posts-go/internal/model"
)

func createTestPost(t *testing.T, s *MemoryStore) *model.Post {
	t.Helper()
	post, err := s.CreatePost(t.Context(), "alice", "заголовок", "текст")
	if err != nil {
		t.Fatal(err)
	}
	return post
}

func TestCreatePost(t *testing.T) {
	s := NewMemoryStore()
	post, err := s.CreatePost(t.Context(), "alice", "мой пост", "текст поста")
	if err != nil {
		t.Fatal(err)
	}
	if post.ID == "" || post.AuthorID != "alice" || post.Title != "мой пост" || post.Body != "текст поста" || !post.CommentsEnabled {
		t.Fatalf("неверные данные поста: %+v", post)
	}
	saved, err := s.Post(t.Context(), post.ID)
	if err != nil {
		t.Fatal(err)
	}
	// разыменовываем и сравниваем, т к все поля сравнимы
	if *saved != *post {
		t.Fatal("сохранённый пост отличается от созданного")
	}
}

func TestCreateComment(t *testing.T) {
	s := NewMemoryStore()
	post := createTestPost(t, s)
	root, err := s.CreateComment(t.Context(), post.ID, nil, "комментарий")
	if err != nil {
		t.Fatal(err)
	}
	if root.ID == "" || root.PostID != post.ID || root.ParentID != nil || root.Body != "комментарий" {
		t.Fatalf("неверный корневой комментарий: %+v", root)
	}
	reply, err := s.CreateComment(t.Context(), post.ID, &root.ID, "ответ")
	if err != nil {
		t.Fatal(err)
	}
	if reply.ID == "" || reply.ID == root.ID || reply.PostID != post.ID || reply.ParentID == nil || *reply.ParentID != root.ID || reply.Body != "ответ" {
		t.Fatalf("неверный ответ: %+v", reply)
	}
	comments, err := s.Comments(t.Context(), post.ID, 10, nil, CommentFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 2 || comments[0].ID != root.ID || comments[1].ID != reply.ID {
		t.Fatal("созданные комментарии не найдены")
	}
}

func TestCommentLength(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		wantError bool
	}{
		{"2000 символов", strings.Repeat("я", 2000), false},
		{"2001 символ", strings.Repeat("я", 2001), true},
		{"пустая строка", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMemoryStore()
			post := createTestPost(t, s)
			comment, err := s.CreateComment(t.Context(), post.ID, nil, tc.body)
			if (err != nil) != tc.wantError {
				t.Fatalf("ошибка: %v, ожидалась: %v", err, tc.wantError)
			}
			if !tc.wantError && comment.Body != tc.body {
				t.Fatal("текст комментария изменился")
			}
		})
	}
}

func TestPostsPagination(t *testing.T) {
	s := NewMemoryStore()
	posts := []*model.Post{createTestPost(t, s), createTestPost(t, s), createTestPost(t, s)}
	for _, tc := range []struct {
		name          string
		limit, offset int32
		want          []graphql.ID
	}{
		{"первая страница", 2, 0, []graphql.ID{posts[0].ID, posts[1].ID}},
		{"смещение и лимит", 1, 1, []graphql.ID{posts[1].ID}},
		{"конец списка", 2, 3, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.Posts(t.Context(), tc.limit, tc.offset)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) != len(tc.want) {
				t.Fatalf("получено %d постов, ожидалось %d", len(page), len(tc.want))
			}
			for i, id := range tc.want {
				if page[i].ID != id {
					t.Fatalf("получен пост %s, ожидался %s", page[i].ID, id)
				}
			}
		})
	}
}

func TestCommentsPagination(t *testing.T) {
	s := NewMemoryStore()
	post := createTestPost(t, s)
	otherPost := createTestPost(t, s)
	var comments []*model.Comment
	for i := 0; i < 3; i++ {
		comment, err := s.CreateComment(t.Context(), post.ID, nil, "комментарий")
		if err != nil {
			t.Fatal(err)
		}
		comments = append(comments, comment)
		if i == 0 {
			if _, err := s.CreateComment(t.Context(), otherPost.ID, nil, "другой пост"); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		name  string
		limit int32
		after *graphql.ID
		want  []graphql.ID
	}{
		{"первая страница", 2, nil, []graphql.ID{comments[0].ID, comments[1].ID}},
		{"после курсора", 1, &comments[1].ID, []graphql.ID{comments[2].ID}},
		{"конец списка", 2, &comments[2].ID, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.Comments(t.Context(), post.ID, tc.limit, tc.after, CommentFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(page) != len(tc.want) {
				t.Fatalf("получено %d комментариев, ожидалось %d", len(page), len(tc.want))
			}
			for i, id := range tc.want {
				if page[i].ID != id {
					t.Fatalf("получен комментарий %s, ожидался %s", page[i].ID, id)
				}
			}
		})
	}
}

func TestCommentsBranches(t *testing.T) {
	s := NewMemoryStore()
	post := createTestPost(t, s)
	otherPost := createTestPost(t, s)
	add := func(postID graphql.ID, parentID *graphql.ID) *model.Comment {
		t.Helper()
		comment, err := s.CreateComment(t.Context(), postID, parentID, "комментарий")
		if err != nil {
			t.Fatal(err)
		}
		return comment
	}
	root := add(post.ID, nil)
	reply := add(post.ID, &root.ID)
	grandchild := add(post.ID, &reply.ID)
	secondReply := add(post.ID, &root.ID)
	secondRoot := add(post.ID, nil)
	otherRoot := add(otherPost.ID, nil)
	add(otherPost.ID, &otherRoot.ID)
	invalidID := graphql.ID("abc")
	// У корня два прямых ответа, у первого ответа — один. Остальные без ответов.
	replyCounts := map[graphql.ID]int32{root.ID: 2, reply.ID: 1}

	for _, tc := range []struct {
		name      string
		filter    CommentFilter
		after     *graphql.ID
		limit     int32
		want      []graphql.ID
		wantError bool
	}{
		{"все комментарии поста", CommentFilter{}, nil, 10, []graphql.ID{root.ID, reply.ID, grandchild.ID, secondReply.ID, secondRoot.ID}, false},
		{"только корни", CommentFilter{RootsOnly: true}, nil, 10, []graphql.ID{root.ID, secondRoot.ID}, false},
		{"только непосредственные ответы", CommentFilter{ParentID: &root.ID}, nil, 10, []graphql.ID{reply.ID, secondReply.ID}, false},
		{"ответ на ответ", CommentFilter{ParentID: &reply.ID}, nil, 10, []graphql.ID{grandchild.ID}, false},
		{"следующая страница корней", CommentFilter{RootsOnly: true}, &root.ID, 1, []graphql.ID{secondRoot.ID}, false},
		{"следующая страница ответов", CommentFilter{ParentID: &root.ID}, &reply.ID, 1, []graphql.ID{secondReply.ID}, false},
		{"нет ответов", CommentFilter{ParentID: &grandchild.ID}, nil, 10, nil, false},
		{"родитель из другого поста", CommentFilter{ParentID: &otherRoot.ID}, nil, 10, nil, false},
		{"несовместимые фильтры", CommentFilter{RootsOnly: true, ParentID: &root.ID}, nil, 10, nil, true},
		{"неверный id родителя", CommentFilter{ParentID: &invalidID}, nil, 10, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.Comments(t.Context(), post.ID, tc.limit, tc.after, tc.filter)
			if (err != nil) != tc.wantError {
				t.Fatalf("ошибка: %v, ожидалась: %v", err, tc.wantError)
			}
			if tc.wantError {
				return
			}
			if len(page) != len(tc.want) {
				t.Fatalf("получено %d комментариев, ожидалось %d", len(page), len(tc.want))
			}
			for i, id := range tc.want {
				if page[i].ID != id {
					t.Fatalf("получен комментарий %s, ожидался %s", page[i].ID, id)
				}
				if page[i].RepliesCount != replyCounts[id] {
					t.Fatalf("комментарий %s: ответов %d, ожидалось %d", id, page[i].RepliesCount, replyCounts[id])
				}
			}
		})
	}
	add(post.ID, &root.ID)
	page, err := s.Comments(t.Context(), post.ID, 1, nil, CommentFilter{RootsOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].RepliesCount != 3 {
		t.Fatal("количество ответов не обновилось после добавления")
	}
}

func TestCommentsSettings(t *testing.T) {
	s := NewMemoryStore()
	post := createTestPost(t, s)
	for _, enabled := range []bool{false, true} {
		updated, err := s.SetCommentsEnabled(t.Context(), post.ID, post.AuthorID, enabled)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := s.Post(t.Context(), post.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.CommentsEnabled != enabled || saved.CommentsEnabled != enabled {
			t.Fatal("настройка комментариев не сохранилась")
		}
		_, err = s.CreateComment(t.Context(), post.ID, nil, "комментарий")
		if (err != nil) != !enabled {
			t.Fatalf("разрешены комментарии: %v, ошибка добавления: %v", enabled, err)
		}
	}
	if _, err := s.SetCommentsEnabled(t.Context(), post.ID, "bob", false); err == nil {
		t.Fatal("другой пользователь смог изменить настройку")
	}
	saved, err := s.Post(t.Context(), post.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.CommentsEnabled {
		t.Fatal("неудачная попытка изменила настройку")
	}
}

func TestMemoryStoreConcurrentReadWrite(t *testing.T) {
	s := NewMemoryStore()
	post := createTestPost(t, s)
	ctx := t.Context()
	if _, err := s.CreateComment(ctx, post.ID, nil, "параллельный комментарий"); err != nil {
		t.Fatal(err)
	}

	const workers = 100
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			if _, err := s.CreateComment(ctx, post.ID, nil, "параллельный комментарий"); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 10; j++ {
				// каждый воркер читает комментарий 10 раз до 100 комментариев
				comments, err := s.Comments(ctx, post.ID, workers, nil, CommentFilter{})
				if err != nil {
					t.Error(err)
					return
				}
				if len(comments) == 0 {
					t.Error("пропал начальный комментарий")
					return
				}
				for _, comment := range comments {
					if comment.ID == "" || comment.PostID != post.ID || comment.Body != "параллельный комментарий" {
						t.Errorf("неверный комментарий при параллельном чтении: %+v", comment)
						return
					}
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	comments, err := s.Comments(ctx, post.ID, workers, nil, CommentFilter{})
	if err != nil {
		t.Fatal(err)
	}

	// добавляем еще страницу так как у нас там лимит 100
	if len(comments) > 0 {
		after := comments[len(comments)-1].ID
		lastPage, err := s.Comments(ctx, post.ID, workers, &after, CommentFilter{})
		if err != nil {
			t.Fatal(err)
		}
		comments = append(comments, lastPage...)
	}
	if len(comments) != workers+1 {
		t.Fatalf("сохранено %d комментариев, ожидалось %d", len(comments), workers+1)
	}
	seen := make(map[graphql.ID]bool)
	for _, comment := range comments {
		if comment.ID == "" || seen[comment.ID] || comment.PostID != post.ID {
			t.Fatalf("неверный или повторяющийся комментарий: %+v", comment)
		}
		seen[comment.ID] = true
	}
}

func TestCommentInvalidReferences(t *testing.T) {
	missingID := graphql.ID("999")
	foreignParentID := graphql.ID("1")
	for _, tc := range []struct {
		name     string
		postID   graphql.ID
		parentID *graphql.ID
	}{
		{"несуществующий пост", missingID, nil},
		{"несуществующий родитель", "1", &missingID},
		{"родитель из другого поста", "1", &foreignParentID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewMemoryStore()
			createTestPost(t, s)
			otherPost := createTestPost(t, s)
			if _, err := s.CreateComment(t.Context(), otherPost.ID, nil, "чужой комментарий"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateComment(t.Context(), tc.postID, tc.parentID, "ответ"); err == nil {
				t.Fatal("ожидалась ошибка связи")
			}
		})
	}
}
