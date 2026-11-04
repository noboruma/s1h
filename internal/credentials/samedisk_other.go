//go:build !unix

package credentials

func SameDisk(a, b string) (same, ok bool) { return false, false }
