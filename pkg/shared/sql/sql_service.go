package sqlservice

type PagingSQL struct {
	Offset   int32  `form:"page" binding:"required,min=1"`
	PageSize int32  `form:"page_size" binding:"required,min=1,max=50"`
	SortBy   string `form:"sort_by"`
	Order    string `form:"order"`
}
