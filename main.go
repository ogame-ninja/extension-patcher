package extension_patcher

import (
	"archive/zip"
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/ogame-ninja/extension-patcher/pkg/stores"
	"github.com/ogame-ninja/extension-patcher/pkg/utils"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const (
	perm                  os.FileMode = 0644
	panicOnReplaceErr                 = true
	jsExtension                       = ".js"
	chromeWebstorePrefix1             = "https://chrome.google.com/webstore/detail/"
	chromeWebstorePrefix2             = "https://chromewebstore.google.com/detail/"
	mozillaWebstorePrefix             = "https://addons.mozilla.org/"
	openUserJSPrefix                  = "https://openuserjs.org/install/"
	githubPrefix                      = "https://github.com/"
)

// Int re-export the Int function
var Int = utils.Int

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
	KeepZip          bool // Either or not to keep the original file
	AutoAnalysis     bool
	Webstore         Webstore
}

type Patcher struct {
	params Params
}

type Webstore interface {
	GetName() string
	GetDownloadLink() string
	ValidatePayload(reader io.Reader) error
}

var ErrInvalidWebstoreURL = errors.New("invalid WebstoreURL")

func NewStore(webstoreURL string) (Webstore, error) {
	if strings.HasPrefix(webstoreURL, chromeWebstorePrefix1) ||
		strings.HasPrefix(webstoreURL, chromeWebstorePrefix2) {
		return &stores.ChromeStore{stores.BaseStore{WebstoreURL: webstoreURL}}, nil
	} else if strings.HasPrefix(webstoreURL, mozillaWebstorePrefix) {
		return &stores.MozillaStore{stores.BaseStore{WebstoreURL: webstoreURL}}, nil
	} else if strings.HasPrefix(webstoreURL, openUserJSPrefix) {
		return &stores.OpenUserJSStore{stores.BaseStore{WebstoreURL: webstoreURL}}, nil
	} else if strings.HasPrefix(webstoreURL, githubPrefix) {
		return &stores.GithubStore{stores.BaseStore{WebstoreURL: webstoreURL}}, nil
	} else if !strings.HasPrefix(webstoreURL, "http") {
		return &stores.FileStore{stores.BaseStore{WebstoreURL: webstoreURL}}, nil
	}
	return nil, ErrInvalidWebstoreURL
}

func New(params Params) (*Patcher, error) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	webstoreURL := params.WebstoreURL

	if params.ExpectedSha256 == "" {
		return nil, errors.New("missing ExpectedSha256")
	}
	if len(params.ExpectedSha256) != 0 && len(params.ExpectedSha256) != 64 {
		return nil, errors.New("ExpectedSha256 must be 64 characters long")
	}
	if webstoreURL == "" {
		return nil, errors.New("missing WebstoreURL")
	}
	webstore, err := NewStore(webstoreURL)
	if err != nil {
		return nil, err
	}
	// No extension name provided, extract it from the webstore url
	if params.ExtensionName == "" {
		params.ExtensionName = webstore.GetName()
	}
	params.Webstore = webstore
	if len(params.Files) == 0 {
		return nil, errors.New("missing Files")
	}
	if params.DelayBeforeClose == nil {
		params.DelayBeforeClose = Int(5)
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
			if r := recover(); r != nil {
				fmt.Println(r, "\n", string(debug.Stack()))
			} else {
				// This is useful on windows because the default CMD window closes immediately
				time.Sleep(time.Duration(*delayBeforeClose) * time.Second)
			}
		}()
	}

	extensionName := p.params.ExtensionName
	webstore := p.params.Webstore
	expectedSha256 := p.params.ExpectedSha256

	_, isOpenUserJSStore := webstore.(*stores.OpenUserJSStore)
	_, isFileStore := webstore.(*stores.FileStore)

	extensionNameZip := extensionName + ".zip"

	if isOpenUserJSStore {
		_ = os.Mkdir(extensionName, 0755)
		extensionNameZip = filepath.Join(extensionName, extensionName+".user.js.orig")
	} else if isFileStore {
		_ = os.Mkdir(extensionName, 0755)
		extensionNameZip = webstore.GetDownloadLink()
	}

	if isFileStore {
	} else if !utils.FileExists(extensionNameZip) {
		if err := downloadExtension(webstore, extensionNameZip); err != nil {
			panic(err)
		}
		fmt.Println("extension downloaded")
	}

	extensionZipSha256 := utils.Sha256f(extensionNameZip)
	if extensionZipSha256 != expectedSha256 {
		fmt.Printf("invalid sha256 for %s (sha256: %s) \n", extensionNameZip, extensionZipSha256)
		return
	}

	if isOpenUserJSStore {
		if err := utils.CopyFile(extensionNameZip, strings.TrimSuffix(extensionNameZip, ".orig")); err != nil {
			panic(err)
		}
	} else {
		if isFileStore && !strings.HasSuffix(extensionNameZip, ".zip") {
			if err := utils.CopyFile(extensionNameZip, filepath.Join(extensionName, extensionNameZip)); err != nil {
				panic(err)
			}
		} else {
			if err := unzip(extensionNameZip, extensionName); err != nil {
				panic(err)
			}
		}
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

	if p.params.JsBeautify && strings.HasSuffix(filename, jsExtension) {
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
		maxLen = max(len(f.FileName)+1, maxLen)
	}
	for _, f := range p.params.Files {
		p.processFile(f.FileName, f.Processors, maxLen)
	}
}

type myEntryStruct struct {
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
	var myEntries []myEntryStruct
	for _, entry := range entries {
		myEntries = append(myEntries, myEntryStruct{DirEntry: entry, Prefix: extName})
	}
	var entry myEntryStruct
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
				myEntries = append(myEntries, myEntryStruct{DirEntry: newEntry, Prefix: filePath})
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

func analyzeFileContent(by []byte, filePath string, entry myEntryStruct, err error) {
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
		processFileContent(by, filePath, entry, termsFound, terms)
	}
}

func processFileContent(by []byte, filePath string, entry myEntryStruct, termsFound []string, terms []string) {
	fmt.Printf("%s\n", Green(filePath))
	fmt.Printf("contains: %s\n", strings.Join(termsFound, ", "))

	// If we are processing a .js file, run js-beautify on it and save it
	if strings.HasSuffix(filePath, jsExtension) {
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
		if shouldProcessLine(line, terms) {
			line = processLine(line, terms)
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

// We should process a line if any of the terms is present in it
func shouldProcessLine(line string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(line, term) {
			return true
		}
	}
	return false
}

// Colorify all the terms in the line
func processLine(line string, terms []string) string {
	var newTerms []string
	for _, term := range terms {
		newTerms = append(newTerms, term, Red(term))
	}
	replacer := strings.NewReplacer(newTerms...)
	return replacer.Replace(line)
}

// NoColor ...
var NoColor = utils.False()

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

func White(val string) string   { return colorStr(kwht, val) }
func Cyan(val string) string    { return colorStr(kcyn, val) }
func Red(val string) string     { return colorStr(kred, val) }
func Blue(val string) string    { return colorStr(kblu, val) }
func Yellow(val string) string  { return colorStr(kyel, val) }
func Green(val string) string   { return colorStr(kgrn, val) }
func Magenta(val string) string { return colorStr(kmag, val) }

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

func downloadExtension(webstore Webstore, zipFileName string) error {
	downloadLink := webstore.GetDownloadLink()

	resp, err := http.Get(downloadLink)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := webstore.ValidatePayload(resp.Body); err != nil {
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
