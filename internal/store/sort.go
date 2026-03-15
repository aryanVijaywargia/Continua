package store

// SortDirection controls ascending or descending ordering.
type SortDirection string

const (
	SortDirectionAsc  SortDirection = "asc"
	SortDirectionDesc SortDirection = "desc"
)

func normalizeSortDirection(direction SortDirection) SortDirection {
	if direction == SortDirectionAsc {
		return SortDirectionAsc
	}
	return SortDirectionDesc
}
