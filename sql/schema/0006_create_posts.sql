-- +goose Up
CREATE TABLE posts (
  id UUID PRIMARY KEY,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP  NOT NULL,
  title VARCHAR(255) NOT NULL,
  url VARCHAR(255) NOT NULL UNIQUE,
  description TEXT NOT NULL,
  feed_id UUID NOT NULL REFERENCES feeds ON DELETE CASCADE,
  published_at TIMESTAMP,
  FOREIGN KEY(feed_id) REFERENCES feeds(id)
);

-- +goose Down
DROP TABLE posts;
