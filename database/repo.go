package database

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is an interface that defines the methods for executing queries and transactions.
// only supports pgx package related methods.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// FindOne executes a query and returns a single row as a struct of type T.
// It uses the given struct type to dynamically generate the query based on the struct's tags.
// It must be given a valid struct with tags, such as:
//
//	type Company struct {
//		TableName     string            `db:"-" table:"companies c"`
//		ID            uuid.UUID         `db:"id" primary:"true"`
//		Name          string            `db:"name"`
//		FlowCount     int               `db:"flow_count" query:"(SELECT count(*) FROM flows f WHERE f.company_id = c.id)"`
//		EmployeeCount int               `db:"employee_count" query:"(SELECT count(*) FROM company_employees ce WHERE ce.company_id = c.id)"`
//		Employees     []CompanyEmployee `db:"employees" query:"(SELECT array_agg(row(ce.*)) FROM company_employees ce WHERE ce.company_id = c.id)"`
//	}
//
// As you can see, the table name is specified in the `table` tag. It must contain an alias.
// the extra fields are specified in the `query` tag, which can be used to eagerly fetch related data such as formulas or child records.
// If the query fails or no rows are returned, it returns an error.
func FindOne[T any](ctx context.Context, dbtx DBTX, options string, args ...any) (T, error) {
	var result T

	baseQuery, err := generateSelectQuery[T]()
	if err != nil {
		return result, fmt.Errorf("failed to generate select query: %w", err)
	}
	query := fmt.Sprintf("%s %s", baseQuery, options)
	result, err = SelectRow[T](ctx, dbtx, query, args...)
	if err != nil {
		return result, fmt.Errorf("failed to execute query: %w", err)
	}

	return result, nil
}

// Find executes a query and returns multiple rows as a slice of structs of type T.
// It uses the given struct type to dynamically generate the query based on the struct's tags.
// It must be given a valid struct with tags, such as:
//
//	type Company struct {
//		TableName     string            `db:"-" table:"companies c"`
//		ID            uuid.UUID         `db:"id" primary:"true"`
//		Name          string            `db:"name"`
//		FlowCount     int               `db:"flow_count" query:"(SELECT count(*) FROM flows f WHERE f.company_id = c.id)"`
//		EmployeeCount int               `db:"employee_count" query:"(SELECT count(*) FROM company_employees ce WHERE ce.company_id = c.id)"`
//		Employees     []CompanyEmployee `db:"employees" query:"(SELECT array_agg(row(ce.*)) FROM company_employees ce WHERE ce.company_id = c.id)"`
//	}
//
// As you can see, the table name is specified in the `table` tag. It must contain an alias.
// the extra fields are specified in the `query` tag, which can be used to eagerly fetch related data such as formulas or child records.
// If the query fails or no rows are returned, it returns an error.
func Find[T any](ctx context.Context, dbtx DBTX, options string, args ...any) ([]T, error) {
	baseQuery, err := generateSelectQuery[T]()
	if err != nil {
		return nil, fmt.Errorf("failed to generate select query: %w", err)
	}
	query := fmt.Sprintf("%s %s", baseQuery, options)
	results, err := SelectRows[T](ctx, dbtx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return results, nil
}

// generateSelectQuery generates a SQL SELECT query based on the struct type T.
func generateSelectQuery[T any]() (string, error) {
	s := *new(T)
	// Get the type and value of the struct
	t := reflect.TypeOf(s)
	if t.Kind() != reflect.Struct {
		return "", fmt.Errorf("expected a struct, but got %T", s)
	}

	var selectClauses []string
	var fromClause string
	tableAlias := ""

	// Iterate over the fields of the struct
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Handle the table name from the `table` tag
		if tableTag := field.Tag.Get("table"); tableTag != "" {
			parts := strings.Split(tableTag, " ")
			if len(parts) > 0 {
				fromClause = tableTag
			}
			if len(parts) > 1 {
				tableAlias = parts[1]
			}
			// Add the main table selector (e.g., "c.*") to the select clauses
			if tableAlias != "" {
				selectClauses = append(selectClauses, fmt.Sprintf("%s.*", tableAlias))
			}
			continue
		}

		// Get the `db` tag for the column name
		dbTag := field.Tag.Get("db")
		if dbTag == "-" {
			// Skip fields that are explicitly ignored
			continue
		}

		// Handle subqueries from the `query` tag
		if queryTag := field.Tag.Get("query"); queryTag != "" {
			// Use the db tag as the alias for the subquery result
			selectClauses = append(selectClauses, fmt.Sprintf("%s AS %s", queryTag, dbTag))
		}
	}

	if fromClause == "" {
		return "", fmt.Errorf("no `table` tag found in struct")
	}

	// Combine all parts into the final query string
	query := fmt.Sprintf("SELECT\n  %s\nFROM %s", strings.Join(selectClauses, ",\n  "), fromClause)

	return query, nil
}

// Save inserts or updates a struct of type T into the database.
// It generates an SQL INSERT query based on the struct's tags and executes it using the provided DBTX.
// Look at the documentation for `FindOne` and `Find` for how to use the struct tags. It will ignore
// fields that is marked with `query` tag or `db` tag with value `-`.
//
// It will generate an INSERT query with ON CONFLICT handling for primary keys by upserting the values if the primary key already exists.
//
// It returns an error if the query generation or execution fails.
func Save[T any](ctx context.Context, dbtx DBTX, data T) error {
	query, values, err := generateInsertQuery(data)
	if err != nil {
		return fmt.Errorf("failed to generate insert query: %w", err)
	}
	if err := ExecQuery(ctx, dbtx, query, values...); err != nil {
		return fmt.Errorf("failed to execute insert query: %w", err)
	}
	return nil
}

// generateInsertQuery generates an SQL INSERT query for the given struct type.
// It returns the query string and a slice of values to be used in the query.
func generateInsertQuery(v any) (string, []any, error) {
	val := reflect.Indirect(reflect.ValueOf(v))
	typ := val.Type()

	if typ.Kind() != reflect.Struct {
		return "", nil, fmt.Errorf("input must be a struct pointer")
	}

	var tableName string
	var columns []string
	var primaryKeys []string
	var values []any

	// First pass: find the table name from the 'table' tag.
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if tag, ok := field.Tag.Lookup("table"); ok {
			// Assumes format "table_name alias"
			tableName = strings.Split(tag, " ")[0]
			break
		}
	}
	if tableName == "" {
		return "", nil, fmt.Errorf("missing 'table' tag in struct")
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		dbTag := field.Tag.Get("db")

		// Skip fields marked with '-' or those with a 'query' tag.
		if dbTag == "" || dbTag == "-" {
			continue
		}
		if _, isQueryField := field.Tag.Lookup("query"); isQueryField {
			continue
		}

		columns = append(columns, dbTag)
		values = append(values, val.Field(i).Interface())

		if primaryTag := field.Tag.Get("primary"); primaryTag == "true" {
			primaryKeys = append(primaryKeys, dbTag)
		}
	}

	if len(primaryKeys) == 0 {
		return "", nil, fmt.Errorf("no primary key found; use the 'primary:\"true\"' tag")
	}

	var sb strings.Builder
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	// INSERT INTO table (col1, col2)
	sb.WriteString(fmt.Sprintf("INSERT INTO %s (%s)\n", tableName, strings.Join(columns, ", ")))
	// VALUES ($1, $2)
	sb.WriteString(fmt.Sprintf("VALUES (%s)\n", strings.Join(placeholders, ", ")))
	// ON CONFLICT (pk_col)
	sb.WriteString(fmt.Sprintf("ON CONFLICT(%s)\n", strings.Join(primaryKeys, ", ")))

	// DO UPDATE SET col1 = EXCLUDED.col1, ...
	var updateSet []string
	pkMap := make(map[string]struct{})
	for _, pk := range primaryKeys {
		pkMap[pk] = struct{}{}
	}

	for _, col := range columns {
		if _, isPk := pkMap[col]; !isPk {
			updateSet = append(updateSet, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
		}
	}

	if len(updateSet) > 0 {
		sb.WriteString(fmt.Sprintf("DO UPDATE SET %s", strings.Join(updateSet, ",\n")))
	} else {
		// This happens if all savable columns are part of the primary key.
		sb.WriteString("DO NOTHING")
	}

	return sb.String(), values, nil
}
