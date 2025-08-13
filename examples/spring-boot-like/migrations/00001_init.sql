-- +goose Up
CREATE TABLE IF NOT EXISTS books (
  id UUID PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT NOT NULL
);

-- +goose Down
DROP TABLE books;