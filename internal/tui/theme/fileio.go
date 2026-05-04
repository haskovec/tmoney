package theme

import "os"

// readFile is a thin wrapper around os.ReadFile. It exists so test
// code can inject in-memory fixtures via ParseFromFile if a future
// task needs that, without surface-area changes.
func readFile(path string) ([]byte, error) { return os.ReadFile(path) }
