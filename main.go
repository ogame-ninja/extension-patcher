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
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	perm              os.FileMode = 0644
	panicOnReplaceErr             = true
)

var InvalidMagicBytesErr = errors.New("invalid magic bytes")

type Processor func([]byte) []byte

type FileAndProcessors struct {
	FileName   string
	Processors []Processor
}

type Params struct {
	ExtensionName    string // infinity
	ExpectedSha256   string // 315738d9184062db0e42deddf6ab64268b4f7c522484892cf0abddf0560f6bcd
	WebstoreURL      string // https://chrome.google.com/webstore/detail/ogame-infinity/hfojakphgokgpbnejoobfamojbgolcbo
	Files            []FileAndProcessors
	JsBeautify       bool // Either or not to run "js-beautify" on js files
	DelayBeforeClose *int
	KeepZip          bool
}

type Patcher struct {
	params Params
}

func New(params Params) (*Patcher, error) {
	if params.ExtensionName == "" {
		return nil, errors.New("missing ExtensionName")
	}
	if params.ExpectedSha256 == "" {
		return nil, errors.New("missing ExpectedSha256")
	}
	if len(params.ExpectedSha256) != 0 && len(params.ExpectedSha256) != 64 {
		return nil, errors.New("ExpectedSha256 must be 64 characters long")
	}
	if params.WebstoreURL == "" {
		return nil, errors.New("missing WebstoreURL")
	}
	if !strings.HasPrefix(params.WebstoreURL, "https://chrome.google.com/webstore/detail/") &&
		!strings.HasPrefix(params.WebstoreURL, "https://chromewebstore.google.com/detail/") {
		return nil, errors.New("invalid WebstoreURL")
	}
	if len(params.Files) == 0 {
		return nil, errors.New("missing Files")
	}
	if params.DelayBeforeClose == nil {
		params.DelayBeforeClose = new(int)
		*params.DelayBeforeClose = 5
	}
	return &Patcher{params: params}, nil
}

func (p *Patcher) Start() {
	delayBeforeClose := p.params.DelayBeforeClose
	if delayBeforeClose != nil {
		defer func() {
			// This is useful on windows because the default CMD window closes immediately
			time.Sleep(time.Duration(*delayBeforeClose) * time.Second)
		}()
	}

	extensionName := p.params.ExtensionName
	webstoreURL := p.params.WebstoreURL
	expectedSha256 := p.params.ExpectedSha256

	extensionNameZip := extensionName + ".zip"

	if !fileExists(extensionNameZip) {
		if err := downloadExtension(webstoreURL, extensionNameZip); err != nil {
			panic(err)
		}
		fmt.Println("extension downloaded")
	}

	extensionZipSha256 := sha256f(extensionNameZip)
	if extensionZipSha256 != expectedSha256 {
		fmt.Printf("invalid file from chrome app store (sha256: %s) \n", extensionZipSha256)
		return
	}

	if err := unzip(extensionNameZip, extensionName); err != nil {
		panic(err)
	}

	if !p.params.KeepZip {
		_ = os.Remove(extensionNameZip)
	}

	p.processFiles()

	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	fmt.Println("Done. code generated in " + path)
}

func (p *Patcher) processFile(filename string, processors []Processor, maxLen int) {
	manifestFileName := p.params.ExtensionName + filename
	by, err := os.ReadFile(manifestFileName)
	if err != nil {
		panic(err)
	}
	for _, processor := range processors {
		by = processor(by)
	}

	if p.params.JsBeautify && strings.HasSuffix(filename, ".js") {
		by = JsBeautify(by)
	}

	if err := os.WriteFile(manifestFileName, by, perm); err != nil {
		panic(err)
	}
	fmt.Printf("%-"+strconv.Itoa(maxLen)+"v patched\n", filename)
}

func (p *Patcher) processFiles() {
	maxLen := 0
	for _, f := range p.params.Files {
		maxLen = int(math.Max(float64(len(f.FileName)+1), float64(maxLen)))
	}
	for _, f := range p.params.Files {
		p.processFile(f.FileName, f.Processors, maxLen)
	}
}

func NewFile(fileName string, processors ...Processor) FileAndProcessors {
	return FileAndProcessors{FileName: fileName, Processors: processors}
}

func JsBeautify(in []byte) []byte {
	cmd := exec.Command("js-beautify", "-q", "-f '-'")
	cmd.Stdin = bytes.NewReader(in)
	processed, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return processed
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

func getExtensionIDFromLink(link string) string {
	link = strings.Trim(link, "/")
	parts := strings.Split(link, "/")
	return parts[len(parts)-1]
}

func buildDownloadLink(extensionID string) string {
	return "https://clients2.google.com/service/update2/crx?" +
		"response=redirect&prodversion=49.0&acceptformat=crx3&" +
		"x=id%3D" + extensionID + "%26installsource%3Dondemand%26uc"
}

func parse(reader io.Reader) error {
	const magicBytes = 0x43723234 // Cr24
	var magic uint32
	if err := binary.Read(reader, binary.BigEndian, &magic); err != nil {
		return err
	}
	if magic != magicBytes {
		return InvalidMagicBytesErr
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
	if err != nil {
		return err
	}
	return nil
}

func unzip(src string, dst string) error {
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
	oldBytes := []byte(old)
	newBytes := []byte(new)
	if n == -1 {
		out = bytes.Replace(in, oldBytes, newBytes, -1)
		return out, len(out)
	}
	replacementLength := len(new)
	for i := 0; i < n; i++ {
		startIdx := bytes.Index(in, oldBytes)
		if startIdx == -1 {
			panic("not replaced properly")
		}
		newOut := bytes.Replace(in, oldBytes, newBytes, 1)
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
