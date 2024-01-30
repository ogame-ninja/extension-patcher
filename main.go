package extension_patcher

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Params struct {
	ExtensionName  string
	ExpectedSha256 string
	WebstoreURL    string
	Files          []FileAndProcessor
	JsBeautify     bool // Either or not to run "js-beautify" on js files
}

const (
	panicOnReplaceErr = true
	print_sha256      = false
)

func Start(params Params) {
	defer func() {
		// This is useful on windows because the default CMD window closes immediately
		time.Sleep(5 * time.Second)
	}()

	extensionName := params.ExtensionName
	webstoreURL := params.WebstoreURL
	expectedSha256 := params.ExpectedSha256
	files := params.Files
	jsBeautify := params.JsBeautify

	extensionNameZip := extensionName + ".zip"

	if !fileExists(extensionNameZip) {
		if err := downloadExtension(webstoreURL, extensionNameZip); err != nil {
			panic(err)
		}
		fmt.Println("extension downloaded")
	}

	extensionZipSha256 := sha256f(extensionNameZip)
	if print_sha256 {
		fmt.Println(extensionZipSha256)
	}
	if extensionZipSha256 != expectedSha256 {
		fmt.Println("invalid file from chrome app store")
		return
	}

	if err := unzip(extensionNameZip, extensionName); err != nil {
		panic(err)
	}

	_ = os.Remove(extensionNameZip)

	processFiles(extensionName, files, jsBeautify)

	path, _ := os.Getwd()
	fmt.Println("Done. code generated in " + path)
}

func sha256f(filename string) string {
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

const perm os.FileMode = 0644

func processFile(extensionName, filename string, processorFn FileProcessor, jsBeautify bool) {
	manifestFileName := extensionName + filename
	by, err := os.ReadFile(manifestFileName)
	if err != nil {
		panic(err)
	}
	by = processorFn(by)

	if jsBeautify && strings.HasSuffix(filename, ".js") {
		cmd := exec.Command("js-beautify", "-q", "-f '-'")
		cmd.Stdin = bytes.NewReader(by)
		nby, err := cmd.Output()
		if err != nil {
			panic(err)
		}
		by = nby
	}

	_ = os.WriteFile(manifestFileName, by, perm)
	fmt.Printf("%-20v patched\n", filename)
}

func processFiles(extensionName string, files []FileAndProcessor, jsBeautify bool) {
	for _, f := range files {
		processFile(extensionName, f.FileName, f.ProcessorFn, jsBeautify)
	}
}

type FileProcessor func([]byte) []byte

type FileAndProcessor struct {
	FileName    string
	ProcessorFn FileProcessor
}

func getExtensionIDFromLink(link string) string {
	link = strings.Trim(link, "/")
	parts := strings.Split(link, "/")
	return parts[len(parts)-1]
}

func buildDownloadLink(extensionID string) string {
	return "https://clients2.google.com/service/update2/crx?response=redirect&prodversion=49.0&acceptformat=crx3&x=id%3D" + extensionID + "%26installsource%3Dondemand%26uc"
}

var InvalidMagixBytesErr = errors.New("invalid magic bytes")

func parse(reader io.Reader) error {
	magic := uint32(0)
	if err := binary.Read(reader, binary.BigEndian, &magic); err != nil {
		return err
	}
	if magic != 0x43723234 { // Cr24
		return InvalidMagixBytesErr
	}
	var version uint32
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil {
		return err
	}
	var headerLength uint32
	if err := binary.Read(reader, binary.LittleEndian, &headerLength); err != nil {
		return err
	}
	buf := make([]byte, headerLength)
	if _, err := reader.Read(buf); err != nil {
		return err
	}
	return nil
}

func downloadExtension(webstoreURL, zipFileName string) error {
	extensionID := getExtensionIDFromLink(webstoreURL)
	downloadLink := buildDownloadLink(extensionID)
	resp, err := http.Get(downloadLink)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := parse(resp.Body); err != nil {
		return err
	}

	// Create the file
	out, err := os.Create(zipFileName)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func unzip(src string, dst string) error {
	var filenames []string
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for f := range r.File {
		dstpath := filepath.Join(dst, r.File[f].Name)
		if !strings.HasPrefix(dstpath, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("%s: illegal file path", src)
		}
		if r.File[f].FileInfo().IsDir() {
			if err := os.MkdirAll(dstpath, os.ModePerm); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(dstpath), os.ModePerm); err != nil {
				return err
			}
			if rc, err := r.File[f].Open(); err != nil {
				return err
			} else {
				defer rc.Close()
				if of, err := os.OpenFile(dstpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, r.File[f].Mode()); err != nil {
					return err
				} else {
					defer of.Close()
					if _, err = io.Copy(of, rc); err != nil {
						return err
					} else {
						of.Close()
						rc.Close()
						filenames = append(filenames, dstpath)
					}
				}
			}
		}
	}
	if len(filenames) == 0 {
		return fmt.Errorf("zip file is empty")
	}
	return nil
}

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// MustReplace replace "n" occurrences of old with new
// If there are fewer occurrences of old than "n", panic
func MustReplace(in []byte, old, new string, n int) (out []byte) {
	out, _ = mustReplace2(in, old, new, n)
	return out
}

// Replace "n" occurrences of old with new
// Return the last index of the replaced text
func mustReplace2(in []byte, old, new string, n int) (out []byte, lastIdx int) {
	oldb := []byte(old)
	newb := []byte(new)
	if n == -1 {
		out = bytes.Replace(in, oldb, newb, -1)
		return out, len(out)
	}
	replacementLength := len(new)
	for i := 0; i < n; i++ {
		startIdx := bytes.Index(in, oldb)
		if startIdx == -1 {
			panic("not replaced properly")
		}
		newOut := bytes.Replace(in, oldb, newb, 1)
		idx := startIdx + replacementLength
		out = append(out, newOut[:idx]...)
		in = newOut[idx:]
		lastIdx = len(out)
	}
	out = append(out, in...)
	return out, lastIdx
}

// MustReplaceN replace all "n" occurrences of old with new
// If there are fewer/more occurrences of old than n, panic
func MustReplaceN(in []byte, old, new string, n int) []byte {
	out, lastIdx := mustReplace2(in, old, new, n)
	count := bytes.Count(out[lastIdx:], []byte(old))
	if count > 0 {
		msg := "more text to replace " + strconv.Itoa(count)
		if panicOnReplaceErr {
			panic(msg)
		} else {
			fmt.Println(msg)
		}
	}
	return out
}

// Helper functions, useful for tests
func mustReplaceStr(in, old, new string, n int) (out string) {
	return string(MustReplace([]byte(in), old, new, n))
}

// Helper functions, useful for tests
func mustReplaceNStr(in, old, new string, n int) string {
	return string(MustReplaceN([]byte(in), old, new, n))
}
