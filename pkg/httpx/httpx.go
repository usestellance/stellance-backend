package httpx

import (
	"net/http"
	"strings"
)

type RouteGroup struct {
	prefix      string
	router      *http.ServeMux
	middlewares []func(http.Handler) http.Handler
}

func NewRouteGroup(router *http.ServeMux, prefix string) *RouteGroup {
	return &RouteGroup{
		prefix:      strings.TrimRight(prefix, "/"),
		router:      router,
		middlewares: []func(http.Handler) http.Handler{},
	}
}

func (rg *RouteGroup) HandleFunc(pattern string, handler http.HandlerFunc) {
	parts := strings.SplitN(pattern, " ", 2)
	method := ""
	path := pattern

	if len(parts) == 2 {
		method = parts[0]
		path = strings.TrimSpace(parts[1])
	}

	fullPattern := method
	if method != "" {
		fullPattern += " "
	}

	if path == "/" || path == "" {
		fullPattern += rg.prefix
	} else {
		fullPattern += rg.prefix + path
	}

	h := http.Handler(handler)
	for i := len(rg.middlewares) - 1; i >= 0; i-- {
		h = rg.middlewares[i](h)
	}

	rg.router.HandleFunc(fullPattern, h.ServeHTTP)
}

func (rg *RouteGroup) AddGroup(path string) *RouteGroup {
	path = strings.Trim(path, "/")
	newPrefix := rg.prefix
	if path != "" {
		newPrefix = rg.prefix + "/" + path
	}

	return &RouteGroup{
		prefix:      newPrefix,
		router:      rg.router,
		middlewares: append([]func(http.Handler) http.Handler{}, rg.middlewares...),
	}
}

func (rg *RouteGroup) Use(middleware func(http.Handler) http.Handler) {
	rg.middlewares = append(rg.middlewares, middleware)
}
