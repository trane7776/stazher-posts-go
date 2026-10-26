package graph

// Схема описывает, какие данные клиент может запросить.
const schemaText = `
	type Query {
		hello: String!
		post(id: ID!): Post!
		posts(limit: Int! = 10, offset: Int! = 0): [Post!]!
		comments(postID: ID!, parentID: ID, rootsOnly: Boolean! = false, limit: Int! = 10, after: ID): [Comment!]!
	}

	type Post {
		id: ID!
		authorID: ID!
		commentsEnabled: Boolean!
		title: String!
		body: String!
	}

	type Comment {
		id: ID!
		postID: ID!
		parentID: ID
		body: String!
		repliesCount: Int!
	}

	type Mutation {
		createPost(title: String!, body: String!): Post!
		createComment(postID: ID!, parentID: ID, body: String!): Comment!
		setCommentsEnabled(postID: ID!, enabled: Boolean!): Post!
	}
`
