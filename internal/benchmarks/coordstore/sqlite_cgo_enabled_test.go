//go:build cgo && sqlite_cgo

package coordstore_test

import (
	"os"
	"testing"

	"github.com/gastownhall/gascity/internal/benchmarks/coordstore"
	sqlitecgo "github.com/gastownhall/gascity/internal/benchmarks/coordstore/adapters/sqlite-cgo"
)

func appendSQLiteCGOAdapter(adapters []adapterFactory) []adapterFactory {
	if os.Getenv("COORDSTORE_SQLITE_CGO") == "" {
		return adapters
	}
	return append(adapters, adapterFactory{
		name:  "sqlite-cgo",
		newFn: func() coordstore.StoreAdapter { return sqlitecgo.New() },
	})
}

func TestSQLiteCGORegistrationWhenEnabled(t *testing.T) {
	t.Setenv("COORDSTORE_SQLITE_CGO", "1")
	for _, adapter := range buildRegisteredAdapters() {
		if adapter.name == "sqlite-cgo" {
			return
		}
	}
	t.Fatalf("sqlite-cgo adapter was not registered when COORDSTORE_SQLITE_CGO=1")
}
