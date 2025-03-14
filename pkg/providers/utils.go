package providers

import (
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func getExtensionIDFromLink(link string) string {
	link = strings.Trim(link, "/")
	parts := strings.Split(link, "/")
	return parts[len(parts)-1]
}

func getName(uri, rgxStr string) string {
	rgx := regexp.MustCompile(rgxStr)
	m := rgx.FindStringSubmatch(uri)
	return m[1]
}

func downloadExtension(provider Provider, dstFileName string) error {
	downloadLink := provider.GetDownloadLink()

	resp, err := http.Get(downloadLink)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := provider.ValidatePayload(resp.Body); err != nil {
		return err
	}

	// Create the file
	out, err := os.Create(dstFileName)
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
