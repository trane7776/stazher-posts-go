package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"stazher-posts-go/internal/graph"
	"stazher-posts-go/internal/model"
	"stazher-posts-go/internal/store"
)

func main() {
	storage := flag.String("storage", "memory", "хранилище GraphQL-сервера: memory или postgres")
	flag.Parse()

	var selectedStore store.Store
	switch *storage {
	case "memory":
		memory := store.NewMemoryStore()
		for _, post := range []model.Post{
			{AuthorID: "tim", Title: "Первый пост", Body: "Первый пост"},
			{AuthorID: "tim", Title: "Второй пост", Body: "Второй пост"},
		} {
			if _, err := memory.CreatePost(context.Background(), post.AuthorID, post.Title, post.Body); err != nil {
				log.Fatal(err)
			}
		}
		selectedStore = memory
	case "postgres":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		postgres, err := store.NewPostgresStore(ctx, os.Getenv("DATABASE_URL"))
		cancel()
		if err != nil {
			log.Fatal(err)
		}
		defer postgres.Close()
		selectedStore = postgres
	default:
		log.Fatalf("неизвестное хранилище %q: используйте memory или postgres", *storage)
	}

	mux := http.NewServeMux()
	mux.Handle("/query", graph.NewHandler(selectedStore))
	log.Printf("GraphQL-сервер: http://localhost:18080/query, хранилище: %s", *storage)
	log.Fatal(http.ListenAndServe(":18080", mux))
}
