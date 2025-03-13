package stores

import "io"

type BaseStore struct {
	WebstoreURL string
}

func (b BaseStore) GetName() string                 { return "" }
func (b BaseStore) GetDownloadLink() string         { return b.WebstoreURL }
func (b BaseStore) ValidatePayload(io.Reader) error { return nil }
