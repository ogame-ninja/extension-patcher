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
	ExtensionName  string // infinity
	ExpectedSha256 string // 315738d9184062db0e42deddf6ab64268b4f7c522484892cf0abddf0560f6bcd
	WebstoreURL    string // https://chrome.google.com/webstore/detail/ogame-infinity/hfojakphgokgpbnejoobfamojbgolcbo
	Files          []FileAndProcessor
	JsBeautify     bool // Either or not to run "js-beautify" on js files
}

const (
	perm              os.FileMode = 0644
	panicOnReplaceErr             = true
	print_sha256                  = false
)

type Patcher struct {
	Params Params
}

func New(params Params) (*Patcher, error) {
	if params.ExtensionName == "" {
		return nil, errors.New("missing ExtensionName")
	}
	if params.ExpectedSha256 == "" {
		return nil, errors.New("missing ExpectedSha256")
	}
	if len(params.ExpectedSha256) != 64 {
		return nil, errors.New("ExpectedSha256 must be 64 characters long")
	}
	if params.WebstoreURL == "" {
		return nil, errors.New("missing WebstoreURL")
	}
	if !strings.HasPrefix(params.WebstoreURL, "https://chrome.google.com/webstore/detail/") {
		return nil, errors.New("invalid WebstoreURL")
	}
	if len(params.Files) == 0 {
		return nil, errors.New("missing Files")
	}
	return &Patcher{Params: params}, nil
}

func (p *Patcher) Start() {
	defer func() {
		// This is useful on windows because the default CMD window closes immediately
		time.Sleep(5 * time.Second)
	}()

	extensionName := p.Params.ExtensionName
	webstoreURL := p.Params.WebstoreURL
	expectedSha256 := p.Params.ExpectedSha256

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

	p.processFiles()

	path, _ := os.Getwd()
	fmt.Println("Done. code generated in " + path)
}

func (p *Patcher) processFile(filename string, processorFn FileProcessor) {
	manifestFileName := p.Params.ExtensionName + filename
	by, err := os.ReadFile(manifestFileName)
	if err != nil {
		panic(err)
	}
	by = processorFn(by)

	if p.Params.JsBeautify && strings.HasSuffix(filename, ".js") {
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

func (p *Patcher) processFiles() {
	for _, f := range p.Params.Files {
		p.processFile(f.FileName, f.ProcessorFn)
	}
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
