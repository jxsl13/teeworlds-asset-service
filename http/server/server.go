package server

import (
	stdsql "database/sql"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"

	"github.com/jxsl13/asset-service/config"
	"github.com/jxsl13/asset-service/http/service"
	sqlc "github.com/jxsl13/asset-service/sql"
)

//go:embed static
var staticFS embed.FS

//go:embed templates
var templateFS embed.FS

// StaticFS returns the embedded static assets as an http.FileSystem
// for serving /static/* in the router.
func StaticFS() http.FileSystem {
	sub, _ := fs.Sub(staticFS, "static")
	return http.FS(sub)
}

// Server holds the dependencies injected at startup and implements api.StrictServerInterface.
type Server struct {
	dao            sqlc.DAO
	fsys           http.Dir
	tmpDir         *os.Root
	maxStorageSize int64
	validator      *Validator
	thumbnailSizes map[string]config.Resolution
	layoutTpl      *template.Template
	itemsTpl       *template.Template
}

// New creates a Server from a *sql.DB and prepared *sqlc.Queries.
// It opens tempUploadPath as a sandboxed os.Root for temporary upload files.
func New(db *stdsql.DB, q *sqlc.Queries, storagePath string, tempUploadPath string, maxStorageSize int64, allowedResolutions map[string][]config.Resolution, maxUploadSizes map[string]int64, thumbnailSizes map[string]config.Resolution) (*Server, error) {
	tmpRoot, err := os.OpenRoot(tempUploadPath)
	if err != nil {
		return nil, err
	}
	layoutTpl, err := template.ParseFS(templateFS, "templates/layout.html")
	if err != nil {
		return nil, fmt.Errorf("parse layout: %w", err)
	}
	itemsTpl, err := template.ParseFS(templateFS, "templates/items.html")
	if err != nil {
		return nil, fmt.Errorf("parse items: %w", err)
	}
	return &Server{
		dao:            sqlc.NewDAO(db, q),
		fsys:           http.Dir(storagePath),
		tmpDir:         tmpRoot,
		maxStorageSize: maxStorageSize,
		validator:      NewValidator(allowedResolutions, maxUploadSizes),
		thumbnailSizes: thumbnailSizes,
		layoutTpl:      layoutTpl,
		itemsTpl:       itemsTpl,
	}, nil
}

// newService constructs a SearchService for the current request.
func (s *Server) newService() *service.SearchService {
	return service.New(s.dao)
}

// Close releases resources held by the server.
func (s *Server) Close() error {
	return s.tmpDir.Close()
}
