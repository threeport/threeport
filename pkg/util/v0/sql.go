package v0

import "database/sql"

// SqlNullInt64 returns a pointer to a sql.NullInt64 value
// that represents NULL if the input pointer is nil.
func SqlNullInt64(input *uint) *sql.NullInt64 {
	if input == nil {
		return &sql.NullInt64{}
	}
	return &sql.NullInt64{
		Int64: int64(*input),
		Valid: true,
	}
}
