CREATE TABLE comments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id),
    parent_id BIGINT,
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
    UNIQUE (post_id, id),
    FOREIGN KEY (post_id, parent_id) REFERENCES comments(post_id, id),
    CHECK (parent_id IS NULL OR parent_id < id)
);
