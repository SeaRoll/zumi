package springbootlike

import (
	"context"
	"fmt"
	"net/http"

	"github.com/SeaRoll/zumi/database"
	"github.com/SeaRoll/zumi/server"
	"github.com/google/uuid"
)

type API interface {
	InitAPI()
}

type api struct {
	service Service
}

func NewAPI(service Service) API {
	return &api{
		service: service,
	}
}

func (a *api) InitAPI() {
	// Get all books
	//
	// gen:tag=Books
	server.AddHandler("GET /api/v1/books", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ctx context.Context `ctx:"context"`
			database.PageRequest
		}

		err := server.ParseRequest(r, &req)
		if err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
			return
		}

		books, err := a.service.GetBooks(req.Ctx, req.PageRequest)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve books: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusOK, books)
	})

	// Get a book by ID
	//
	// This handler retrieves a book by its ID from the path parameter.
	//
	// gen:tag=Books
	server.AddHandler("GET /api/v1/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ctx context.Context `ctx:"context"`
			ID  uuid.UUID       `path:"id"`
		}

		err := server.ParseRequest(r, &req)
		if err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
			return
		}

		book, err := a.service.GetBookByID(req.Ctx, req.ID)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve book: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusOK, book)
	})

	// Add a book
	//
	// gen:tag=Books
	server.AddHandler("POST /api/v1/books", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ctx  context.Context `ctx:"context"`
			Book NewBookDTO      `body:"json"`
		}

		err := server.ParseRequest(r, &req)
		if err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
			return
		}

		book, err := a.service.CreateBook(req.Ctx, req.Book)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to add book: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusCreated, book)
	})

	// Delete a book
	//
	// gen:tag=Books
	server.AddHandler("DELETE /api/v1/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ctx context.Context `ctx:"context"`
			ID  uuid.UUID       `path:"id"`
		}

		err := server.ParseRequest(r, &req)
		if err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
			return
		}

		err = a.service.DeleteBookByID(req.Ctx, req.ID)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete book: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusAccepted, nil)
	})
}
