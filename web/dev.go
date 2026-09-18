//go:build !production

package web

import "io/fs"

// Development uses Vite's proxy and HMR; production assets come from the production build tag.
func Assets() fs.FS { return nil }
