// Command go generate regenerates the OpenAPI-derived code from
// openapi/snarvei.yaml (repo root). Run from apps/server with the tools from
// .mise.toml on PATH:
//
//	go generate ./...
//
// Generated code is committed; CI and the image never run codegen.
// internal/api/snarvei.yaml is a committed COPY of the root spec because
// go:embed cannot reach above the module root. Never hand-edit it.
package server

//go:generate cp ../../openapi/snarvei.yaml internal/api/snarvei.yaml
//go:generate oapi-codegen -config internal/api/gen/cfg-types.yaml ../../openapi/snarvei.yaml
//go:generate oapi-codegen -config internal/api/gen/cfg-server.yaml ../../openapi/snarvei.yaml
//go:generate sqlc generate
