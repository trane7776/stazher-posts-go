package graph

import (
	"net/http"

	graphql "github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"

	"stazher-posts-go/internal/store"
)

func NewHandler(storage store.Store) http.Handler {
	schema := graphql.MustParseSchema(schemaText, newResolver(storage), graphql.UseFieldResolvers())
	return identifyUser(&relay.Handler{Schema: schema})
}
