package stores

import (
	"regexp"
	"strings"
)

func getExtensionIDFromLink(link string) string {
	link = strings.Trim(link, "/")
	parts := strings.Split(link, "/")
	return parts[len(parts)-1]
}

func getName(webstoreURL, rgxStr string) string {
	rgx := regexp.MustCompile(rgxStr)
	m := rgx.FindStringSubmatch(webstoreURL)
	return m[1]
}
