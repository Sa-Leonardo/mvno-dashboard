package web

import "embed"

// FS contains the report UI served by the Go backend.
//
//go:embed *.html assets/*
var FS embed.FS
