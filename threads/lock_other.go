//go:build !unix

package threads

// lock is a no-op where there is no flock: kitty runs on unix only, so
// nothing writes the history there.
func lock(string) (func(), error) {
	return func() {}, nil
}
