package pgvector

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	pgv "github.com/pgvector/pgvector-go"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
)

var (
	ErrEmbedderWrongNumberVectors = errors.New("number of vectors from embedder does not match number of documents")
	ErrInvalidScoreThreshold      = errors.New("score threshold must be between 0 and 1")
	ErrInvalidFilters             = errors.New("invalid filters")
	ErrUnsupportedOptions         = errors.New("unsupported options")
)

type ColumnValue struct {
	Name  string
	Value any
}

type OptionFilter struct {
	TenantIDColumn ColumnValue
	RolesColumn    ColumnValue
}

func (option OptionFilter) GetRoles() []string {
	return option.RolesColumn.Value.([]string)
}
func (option OptionFilter) GetTenantID() string {
	return option.TenantIDColumn.Value.(string)
}

type AdditionnalValues struct {
	DocumentIDColumn ColumnValue
	TenantIDColumn   ColumnValue
}

func (option AdditionnalValues) GetDocumentID() string {
	return option.DocumentIDColumn.Value.(string)
}
func (option AdditionnalValues) GetTenantID() string {
	return option.TenantIDColumn.Value.(string)
}

type Store struct {
	embedder embeddings.Embedder
	pool     *pgxpool.Pool

	// name of the table
	tableName string

	//name of the column in which text data will be stored. this will come from the "PageContent" of the langchain doc
	textColumnName string

	//name of the column in which embedded vector data will be stored. the data from "PageContent" of the langchain doc will go through the embedding (vector creation) process
	embeddingStoreColumnName string

	//if true, the langchain doc Metadata will be saved to postgresql as well. in that case the, column(s) needs to exist in advance
	saveMetadata bool

	// attributes for similarity search
	//searchKey       string   // name of the column whose value needs to returned by search

	//optional - data for these columns will be added to resulting langchain doc Metadata
	QueryAttributes []string
}

func New(pgConnectionString, tableName, embeddingStoreColumnName, textColumnName string, saveMetadata bool, embedder embeddings.Embedder) (Store, error) {
	//connection string example - postgres://postgres:postgres@localhost/postgres
	pool, err := pgxpool.New(context.Background(), pgConnectionString)

	if err != nil {
		return Store{}, err
	}

	return Store{embedder: embedder,
		tableName:                tableName,
		embeddingStoreColumnName: embeddingStoreColumnName,
		textColumnName:           textColumnName,
		pool:                     pool,
		saveMetadata:             saveMetadata}, nil
}

func (store Store) AddDocuments(ctx context.Context, docs []schema.Document, options ...vectorstores.Option) ([]string, error) {

	texts := make([]string, 0, len(docs))
	for _, doc := range docs {
		texts = append(texts, doc.PageContent)
	}

	vectors, err := store.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return []string{}, err
	}

	if len(vectors) != len(docs) {
		return []string{}, ErrEmbedderWrongNumberVectors
	}

	metadatas := make([]map[string]any, 0, len(docs))

	for i := 0; i < len(docs); i++ {
		metadata := make(map[string]any, len(docs[i].Metadata))
		for key, value := range docs[i].Metadata {
			metadata[key] = value
		}

		opts := store.getOptions(options...)
		if opts.Filters != nil {
			filter := opts.Filters.(AdditionnalValues)
			metadata[filter.DocumentIDColumn.Name] = filter.DocumentIDColumn.Value
			metadata[filter.TenantIDColumn.Name] = filter.TenantIDColumn.Value
		}

		metadatas = append(metadatas, metadata)
	}

	for i, doc := range docs {

		data := map[string]any{}
		data[store.embeddingStoreColumnName] = pgv.NewVector(vectors[i])
		data[store.textColumnName] = doc.PageContent

		metadata := metadatas[i]

		query, values := store.generateInsertQueryWithValues(data, metadata)

		_, err := store.pool.Exec(context.Background(), query, values...)
		if err != nil {
			return []string{}, err
		}
	}

	return texts, nil
}

// getOptions applies given options to default Options and returns it
// This uses options pattern so clients can easily pass options without changing function signature.
func (s Store) getOptions(options ...vectorstores.Option) vectorstores.Options {
	opts := vectorstores.Options{}
	for _, opt := range options {
		opt(&opts)
	}
	return opts
}

func (store Store) generateInsertQueryWithValues(data, metadata map[string]any) (string, []any) {

	//INSERT INTO test_table (data, embedding) VALUES ($1, $2)
	//INSERT INTO test_table (data, embedding, other_data) VALUES ($1, $2, $3)

	// generate column names and placeholders dynamically
	var columns []string
	var placeholders []string
	var values []any

	for column, value := range data {
		columns = append(columns, column)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(placeholders)+1))
		values = append(values, value)
	}

	if store.saveMetadata {
		for column, value := range metadata {
			switch column {
			case "document_id", "tenant_id":
				columns = append(columns, column)
				placeholders = append(placeholders, fmt.Sprintf("$%d", len(placeholders)+1))
				values = append(values, value)
			default:
			}
		}
	}

	sqlQuery := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		store.tableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	return sqlQuery, values
}

func (store Store) SimilaritySearch(ctx context.Context, searchString string, numDocuments int, options ...vectorstores.Option) ([]schema.Document, error) {

	//fmt.Println("similarity search for", searchString, "with max docs", numDocuments)

	vector, err := store.embedder.EmbedQuery(ctx, searchString)
	if err != nil {
		return nil, err
	}

	roles := []string{}
	tenantID := ""
	opts := store.getOptions(options...)
	if opts.Filters != nil {
		filter := opts.Filters.(OptionFilter)
		roles = filter.GetRoles()
		tenantID = filter.GetTenantID()
	}

	query, err := store.generateSelectQuery(numDocuments, opts.ScoreThreshold, tenantID, roles)
	if err != nil {
		return nil, err
	}

	rows, err := store.pool.Query(ctx, query, pgv.NewVector(vector))

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	docs := []schema.Document{}
	doc := schema.Document{}

	for rows.Next() {
		vals, err := rows.Values()

		if err != nil {
			return nil, err
		}

		doc.PageContent = vals[0].(string)

		score := vals[1].(float64)
		doc.Score = float32(score)

		metadata := make(map[string]any)
		for i := 2; i <= len(vals)-1; i++ {
			metadata[store.QueryAttributes[i-2]] = vals[i]
		}

		doc.Metadata = metadata

		docs = append(docs, doc)
	}

	return docs, nil
}

const baseQueryFormat = `
SELECT %s, 1 - (%s <=> $1) as similarity_score 
FROM %s LEFT JOIN docu_documents ON %s.document_id = docu_documents.id 
WHERE 1 - (embedding <=> $1) > %v AND docu_documents.acl && array['%s'] 
AND %s.tenant_id = '%s'
ORDER BY similarity_score DESC LIMIT %d`

func (store Store) generateSelectQuery(numDocuments int, threshold float32, tenantID string, roles []string) (string, error) {
	var queryBuilder strings.Builder
	var selectedColumns string

	// Ensure required fields are set
	if store.textColumnName == "" || store.embeddingStoreColumnName == "" || store.tableName == "" {
		return "", errors.New("missing essential store fields")
	}

	// Define selected columns based on presence of QueryAttributes
	if len(store.QueryAttributes) > 0 {
		selectedColumns = fmt.Sprintf("%s, %s", store.textColumnName, strings.Join(store.QueryAttributes, ", "))
	} else {
		selectedColumns = store.textColumnName
	}

	// Build query using strings.Builder
	fmt.Fprintf(&queryBuilder, baseQueryFormat,
		selectedColumns,                // Columns
		store.embeddingStoreColumnName, // Similarity comparison
		store.tableName,                // Table name
		store.tableName,                // Join condition
		threshold,                      // Similarity threshold
		strings.Join(roles, "','"),     // Roles for ACL check
		store.tableName,                // Table name
		tenantID,                       // Tenant ID
		numDocuments,                   // Limit on results
	)
	return queryBuilder.String(), nil
}
