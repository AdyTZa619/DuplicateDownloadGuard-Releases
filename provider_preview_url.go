package main

import (
	"fmt"
	"net/http"
	"strings"
)

func providerPreviewPathV86(id int) string {
	return fmt.Sprintf("/api/provider-preview/media?id=%d", id)
}

func providerPreviewURLForRequestV86(r *http.Request, id int) string {
	path := providerPreviewPathV86(id)
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return path
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + strings.TrimSpace(r.Host) + path
}
