```go
package main

import (
	ep "github.com/ogame-ninja/extension-patcher"
)

func main() {
	ep.MustNew(ep.Params{
		Uri: "https://chromewebstore.google.com/detail/ogame-tracker/gcebldjabjlagnnnjfodjgiddnonehnd",
		ExpectedSha256: "",
		Files: []ep.FileAndProcessors{
			ep.NewFile("/manifest.json", processManifest),
		},
	}).Start()
}

var replN = ep.MustReplaceN

func processManifest(by []byte) []byte {
	return by
}
```

```
go mod init your_extension_name
go mod tidy
go mod vendor
go run main.go
```