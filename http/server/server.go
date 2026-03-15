package server

import (
	stdsql "database/sql"
	"net/http"
	"os"

	"github.com/jxsl13/search-service/config"
	"github.com/jxsl13/search-service/http/service"
	sqlc "github.com/jxsl13/search-service/sql"
)

// Server holds the dependencies injected at startup and implements api.StrictServerInterface.
type Server struct {
	dao            sqlc.DAO
	fsys           http.Dir
	tmpDir         *os.Root
	maxStorageSize int64
	validator      *Validator
	thumbnailSize  config.Resolution
}

// New creates a Server from a *sql.DB and prepared *sqlc.Queries.
// It opens tempUploadPath as a sandboxed os.Root for temporary upload files.
func New(db *stdsql.DB, q *sqlc.Queries, storagePath string, tempUploadPath string, maxStorageSize int64, allowedResolutions map[string][]config.Resolution, maxUploadSizes map[string]int64, thumbnailSize config.Resolution) (*Server, error) {
	tmpRoot, err := os.OpenRoot(tempUploadPath)
	if err != nil {
		return nil, err
	}
	return &Server{
		dao:            sqlc.NewDAO(db, q),
		fsys:           http.Dir(storagePath),
		tmpDir:         tmpRoot,
		maxStorageSize: maxStorageSize,
		validator:      NewValidator(allowedResolutions, maxUploadSizes),
		thumbnailSize:  thumbnailSize,
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
