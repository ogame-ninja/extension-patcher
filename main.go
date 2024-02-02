package extension_patcher

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	AutoAnalysis     bool
}

type Patcher struct {
	params Params
}

func New(params Params) (*Patcher, error) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

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
	// No extension name provided, extract it from the webstore url
	if params.ExtensionName == "" {
		rgx := regexp.MustCompile(`/detail/([^/]+)/`)
		m := rgx.FindStringSubmatch(params.WebstoreURL)
		params.ExtensionName = m[1]
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

func MustNew(params Params) *Patcher {
	p, err := New(params)
	if err != nil {
		panic(err)
	}
	return p
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

	if p.params.AutoAnalysis {
		p.autoAnalyse()
	}

	p.processFiles()

	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	fmt.Println("Done. code generated in " + path)
}

func (p *Patcher) processFile(filename string, processors []Processor, maxLen int) {
	filePath := filepath.Join(p.params.ExtensionName, filename)
	by, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	for _, processor := range processors {
		by = processor(by)
	}

	if p.params.JsBeautify && strings.HasSuffix(filename, ".js") {
		by = JsBeautify(by)
	}

	if err := os.WriteFile(filePath, by, perm); err != nil {
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

type MyEntry struct {
	os.DirEntry
	Prefix string
}

func (p *Patcher) autoAnalyse() {
	fmt.Println(strings.Repeat("-", 80))
	extName := p.params.ExtensionName
	entries, err := os.ReadDir(extName)
	if err != nil {
		panic(err)
	}
	var myEntries []MyEntry
	for _, entry := range entries {
		myEntries = append(myEntries, MyEntry{DirEntry: entry, Prefix: extName})
	}
	var entry MyEntry
	for {
		if len(myEntries) == 0 {
			break
		}
		entry, myEntries = myEntries[0], myEntries[1:]
		filePath := filepath.Join(entry.Prefix, entry.Name())
		if entry.IsDir() {
			newEntries, err := os.ReadDir(filePath)
			if err != nil {
				log.Println(err)
				continue
			}
			for _, newEntry := range newEntries {
				myEntries = append(myEntries, MyEntry{DirEntry: newEntry, Prefix: filePath})
			}
			continue
		}
		by, err := os.ReadFile(filePath)
		if err != nil {
			log.Println(err)
			continue
		}
		analyzeFileContent(by, filePath, entry, err)
	}
	fmt.Println(strings.Repeat("-", 80))
}

func analyzeFileContent(by []byte, filePath string, entry MyEntry, err error) {
	terms := []string{
		"index.php",
		"ogame.gameforge.com",
		"alliances.xml",
		"players.xml",
		"universe.xml",
		"highscore.xml",
		"location.host",
	}
	var termsFound []string
	for _, term := range terms {
		if bytes.Contains(by, []byte(term)) {
			termsFound = append(termsFound, Yellow(term))
		}
	}
	if len(termsFound) > 0 {
		fmt.Printf("%s\n", Green(filePath))
		fmt.Printf("contains: %s\n", strings.Join(termsFound, ", "))
		if strings.HasSuffix(filePath, ".js") {
			by = JsBeautify(by)
			info, _ := entry.DirEntry.Info()
			if err := os.WriteFile(filePath, by, info.Mode()); err != nil {
				log.Println(err)
			}
		}
		scanner := bufio.NewScanner(bytes.NewReader(by))
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, len(by))
		lineNumber := 0
		for scanner.Scan() {
			lineNumber++
			line := scanner.Text()

			shouldProcessLine := false
			for _, term := range terms {
				if strings.Contains(line, term) {
					shouldProcessLine = true
					break
				}
			}
			if shouldProcessLine {
				var newTerms []string
				for _, term := range terms {
					newTerms = append(newTerms, term, Red(term))
				}
				replacer := strings.NewReplacer(newTerms...)
				line = replacer.Replace(line)
				fmt.Printf("%d: %s\n", lineNumber, line)
			}
		}
		if err := scanner.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				log.Printf("%d: %v\n", lineNumber+1, err)
			} else {
				log.Println(err)
			}
		}
		fmt.Println(strings.Repeat("-", 30))
	}
}

// NoColor ...
var NoColor = false

// Terminal styling constants
const (
	knrm = "\x1B[0m"
	kred = "\x1B[31m"
	kgrn = "\x1B[32m"
	kyel = "\x1B[33m"
	kblu = "\x1B[34m"
	kmag = "\x1B[35m"
	kcyn = "\x1B[36m"
	kwht = "\x1B[37m"
)

func colorStr(color string, val string) string {
	if NoColor {
		return val
	}
	return color + val + knrm
}

// White ...
func White(val string) string {
	return colorStr(kwht, val)
}

// Cyan ...
func Cyan(val string) string {
	return colorStr(kcyn, val)
}

// Red ...
func Red(val string) string {
	return colorStr(kred, val)
}

// Blue ...
func Blue(val string) string {
	return colorStr(kblu, val)
}

// Yellow ...
func Yellow(val string) string {
	return colorStr(kyel, val)
}

// Green ...
func Green(val string) string {
	return colorStr(kgrn, val)
}

// Magenta ...
func Magenta(val string) string {
	return colorStr(kmag, val)
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
	newBytes = bytes.Replace(newBytes, []byte("{old}"), oldBytes, 1)
	if n == -1 {
		out = bytes.Replace(in, oldBytes, newBytes, -1)
		return out, len(out)
	}
	replacementLength := len(new)
	for i := 0; i < n; i++ {
		startIdx := bytes.Index(in, oldBytes)
		if startIdx == -1 {
			panic(fmt.Sprintf("expected %d replacements, did %d", n, i))
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

func Int(v int) *int {
	tmp := v
	return &tmp
}
