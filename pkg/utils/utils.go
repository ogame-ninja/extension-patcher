package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// Ptr returns a pointer to the given argument
func Ptr[T any](v T) *T {
	return &v
}

// Int returns a pointer to the given int
func Int(v int) *int {
	return Ptr(v)
}

// FileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func FileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// CopyFile copies a src file to dst
func CopyFile(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// Sha256f computes the sha256 of a file
func Sha256f(filename string) string {
	h := sha256.New()
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err := io.Copy(h, f); err != nil {
		panic(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func True() bool  { return true }
func False() bool { return false }
