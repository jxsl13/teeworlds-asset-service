package api

//go:generate rm -f server.gen.go types.gen.go
//go:generate go tool oapi-codegen --config types.cfg.yaml openapi.yaml
//go:generate go tool oapi-codegen --config server.cfg.yaml openapi.yaml
