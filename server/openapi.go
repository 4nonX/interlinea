package server

import (
	_ "embed"
	"net/http"
)

// openAPIDoc describes the API. cmd/interlinea-apidoc writes it from the
// routes, the handlers' doc comments and the types they read and write;
// CI checks that it is up to date.
//
//go:embed openapi.json
var openAPIDoc []byte

// openAPI returns this description of the API, in OpenAPI 3.1.
//
// Access: anyone.
func (s *Server) openAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(openAPIDoc)
}
