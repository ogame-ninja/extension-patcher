package utils

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// Ternary ...
func Ternary[T any](predicate bool, a, b T) T {
	if predicate {
		return a
	}
	return b
}

// Or return "a" if it is non-zero otherwise "b"
func Or[T comparable](a, b T) (zero T) {
	return Ternary(a != zero, a, b)
}

func First[T any](a T, _ ...any) T { return a }

func Unzip(src string, dst string) error {
	var filenames []string
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for f := range r.File {
		if err := func(f int) error {
			destinationPath := filepath.Join(dst, r.File[f].Name)
			if !strings.HasPrefix(destinationPath, filepath.Clean(dst)+string(os.PathSeparator)) {
				return fmt.Errorf("%s: illegal file path", src)
			}
			if r.File[f].FileInfo().IsDir() {
				if err := os.MkdirAll(destinationPath, os.ModePerm); err != nil {
					return err
				}
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(destinationPath), os.ModePerm); err != nil {
				return err
			}
			rc, err := r.File[f].Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			of, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, r.File[f].Mode())
			if err != nil {
				return err
			}
			defer of.Close()
			if _, err = io.Copy(of, rc); err != nil {
				return err
			}
			filenames = append(filenames, destinationPath)
			return nil
		}(f); err != nil {
			return err
		}
	}
	if len(filenames) == 0 {
		return fmt.Errorf("zip file is empty")
	}
	return nil
}

// Must ...
func Must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}

// CheckErr ...
func CheckErr(err error) {
	if err != nil {
		panic(err)
	}
}
