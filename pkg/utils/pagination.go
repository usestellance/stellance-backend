package utils

import (
	"net/http"
	"strconv"
)

// PageMeta is the pagination block returned by every list endpoint.
type PageMeta struct {
	Page       int `json:"page" example:"1"`        // Current page, from 1
	Limit      int `json:"limit" example:"20"`      // Page size
	Total      int `json:"total" example:"57"`      // Total matching rows
	TotalPages int `json:"total_pages" example:"3"` // Total pages
}

func NewPageMeta(page, limit, total int) PageMeta {
	pages := 0
	if limit > 0 {
		pages = (total + limit - 1) / limit
	}
	return PageMeta{Page: page, Limit: limit, Total: total, TotalPages: pages}
}

// PageSizeParam reads the page size from "limit", falling back to a legacy parameter name.
func PageSizeParam(r *http.Request, legacy string) (int, bool) {
	for _, k := range []string{"limit", legacy} {
		if v := r.URL.Query().Get(k); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}
