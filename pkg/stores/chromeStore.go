package stores

import (
	"encoding/binary"
	"errors"
	"io"
)

type ChromeStore struct {
	BaseStore
}

func (s *ChromeStore) GetDownloadLink() string {
	extensionID := getExtensionIDFromLink(s.WebstoreURL)
	return buildDownloadLink(extensionID)
}

func (s *ChromeStore) GetName() string {
	return getName(s.WebstoreURL, `/detail/([^/]+)/`)
}

func (s *ChromeStore) ValidatePayload(reader io.Reader) error {
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

func buildDownloadLink(extensionID string) string {
	return "https://clients2.google.com/service/update2/crx?" +
		"response=redirect&prodversion=131.0.0.0&acceptformat=crx3&" +
		"x=id%3D" + extensionID + "%26installsource%3Dondemand%26uc"
}

var InvalidMagicBytesErr = errors.New("invalid magic bytes")
