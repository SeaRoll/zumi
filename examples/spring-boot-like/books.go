package springbootlike

import "github.com/google/uuid"

type Book struct {
	ID          uuid.UUID `db:"id"`
	Title       string    `db:"title"`
	Description string    `db:"description"`
}

type NewBookDTO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type BookDTO struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
}
