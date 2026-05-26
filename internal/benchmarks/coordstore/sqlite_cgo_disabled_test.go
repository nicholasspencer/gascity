//go:build !(cgo && sqlite_cgo)

package coordstore_test

func appendSQLiteCGOAdapter(adapters []adapterFactory) []adapterFactory {
	return adapters
}
