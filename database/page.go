// This file has everything related to the database connection and migrations.

package database

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
)

type PageRequest struct {
	Page int      `query:"page" json:"page"` // The requested page number (0-indexed)
	Size int      `query:"size" json:"size"` // The number of items per page
	Sort []string `query:"sort" json:"sort"` // A slice of sort commands, e.g., "name,desc"
}

type Page[T any] struct {
	Content          []T         `json:"content"`
	Pageable         PageRequest `json:"pageable"`
	TotalElements    int64       `json:"totalElements"`
	TotalPages       int         `json:"totalPages"`
	Number           int         `json:"number"`
	Size             int         `json:"size"`
	NumberOfElements int         `json:"numberOfElements"`
	IsLast           bool        `json:"last"`
	IsFirst          bool        `json:"first"`
	IsEmpty          bool        `json:"empty"`
}

func MapContent[T any, R any](page Page[T], newContent []R) Page[R] {
	return Page[R]{
		Content:          newContent,
		Pageable:         page.Pageable,
		TotalElements:    page.TotalElements,
		TotalPages:       page.TotalPages,
		Number:           page.Number,
		Size:             page.Size,
		NumberOfElements: page.NumberOfElements,
		IsLast:           page.IsLast,
		IsFirst:          page.IsFirst,
		IsEmpty:          page.IsEmpty,
	}
}

// SelectRowsPageable executes a query and returns a paginated result set.
// It uses the provided dbtx to execute the query and collects the results into a Page[T].
// The PageRequest parameter specifies the pagination details such as page number and size.
//
// Note: make sure the query does not have a `;` at the end, as this function appends LIMIT and OFFSET for pagination.
func SelectRowsPageable[T any](
	ctx context.Context,
	dbtx DBTX,
	pageRequest PageRequest,
	query string,
	args ...any,
) (Page[T], error) {
	var page Page[T]

	page.Pageable = pageRequest

	// Calculate offset and limit
	offset := pageRequest.Page * pageRequest.Size
	limit := pageRequest.Size

	// Add Sorting if provided
	if len(pageRequest.Sort) > 0 { //nolint:nestif
		for i, sort := range pageRequest.Sort {
			if sort == "" {
				continue
			}

			// check if , is present to determine if it's ascending or descending
			splitSort := strings.Split(sort, ",")
			if len(splitSort) == 2 && (splitSort[1] == "asc" || splitSort[1] == "desc") {
				if i == 0 {
					query += fmt.Sprintf(" ORDER BY %s %s", splitSort[0], splitSort[1])
				} else {
					query += fmt.Sprintf(", %s %s", splitSort[0], splitSort[1])
				}
			} else {
				if i == 0 {
					query += " ORDER BY " + sort
				} else {
					query += ", " + sort
				}
			}
		}
	}

	// Modify the query to include pagination
	query += " LIMIT $1 OFFSET $2"

	args = append(args, limit, offset)

	slog.Info("Executing paginated query",
		"query", query,
		"args", args,
	)

	// Execute the query
	rows, err := dbtx.Query(ctx, query, args...)
	if err != nil {
		return page, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	// Collect results
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return page, fmt.Errorf("failed to collect rows: %w", err)
	}

	page.Content = results
	page.Number = pageRequest.Page
	page.Size = pageRequest.Size
	page.NumberOfElements = len(results)
	page.IsEmpty = len(results) == 0

	// Get total elements and pages
	countQuery := "SELECT COUNT(*) FROM (" + query + ") AS count_query"

	var totalElements int64

	err = dbtx.QueryRow(ctx, countQuery, args...).Scan(&totalElements)
	if err != nil {
		return page, fmt.Errorf("failed to get total elements: %w", err)
	}

	page.TotalElements = totalElements
	page.TotalPages = int((totalElements + int64(pageRequest.Size) - 1) / int64(pageRequest.Size))
	page.IsLast = page.Number >= page.TotalPages-1
	page.IsFirst = page.Number == 0

	return page, nil
}
