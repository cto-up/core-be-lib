package helpers

import sqlservice "ctoup.com/coreapp/pkg/shared/sql"

type PagingRequest struct {
	Page            *int32 `form:"page" binding:"required,min=1"`
	PageSize        *int32 `form:"page_size" binding:"required,min=1,max=50"`
	SortBy          *string
	Order           *string
	MaxPageSize     int32
	DefaultPage     int32
	DefaultPageSize int32
	DefaultSortBy   string
	DefaultOrder    string
}

func GetPagingSQL(pagingRequest PagingRequest) sqlservice.PagingSQL {

	pageSize := pagingRequest.DefaultPageSize
	if pagingRequest.PageSize != nil {
		if *pagingRequest.PageSize > pagingRequest.MaxPageSize {
			pageSize = pagingRequest.MaxPageSize
		} else {
			pageSize = *pagingRequest.PageSize
		}
	}

	// Calculate offset based on page number
	offset := int32(0)
	if pagingRequest.Page != nil && *pagingRequest.Page > 1 {
		offset = pageSize * (*pagingRequest.Page - 1)
	}

	sortBy := pagingRequest.DefaultSortBy
	if pagingRequest.SortBy != nil && *pagingRequest.SortBy != "" {
		sortBy = *pagingRequest.SortBy
	}

	order := "asc"
	if pagingRequest.Order != nil && *pagingRequest.Order != "asc" {
		order = "desc"
	}

	return sqlservice.PagingSQL{
		Offset:   offset,
		PageSize: pageSize,
		SortBy:   sortBy,
		Order:    order,
	}
}
