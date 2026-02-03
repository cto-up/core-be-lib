package db

import (
	"ctoup.com/coreapp/pkg/core/db/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Provides all function  to execute db queries and transactions
type Store struct {
	*repository.Queries
	ConnPool *pgxpool.Pool
}

func NewStore(connPool *pgxpool.Pool) *Store {
	return &Store{
		Queries:  repository.New(connPool),
		ConnPool: connPool,
	}
}
