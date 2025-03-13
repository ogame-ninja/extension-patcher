package stores

func NewMozillaStore(webstoreURL string) *MozillaStore {
	return &MozillaStore{BaseStore: BaseStore{WebstoreURL: webstoreURL}}
}

type MozillaStore struct {
	BaseStore
}

func (s *MozillaStore) GetName() string {
	return getName(s.WebstoreURL, `/addon/([^/]+)/?`)
}

func (s *MozillaStore) GetDownloadLink() string {
	extensionID := getExtensionIDFromLink(s.WebstoreURL)
	return "https://addons.mozilla.org/firefox/downloads/latest/" + extensionID + "/platform:3/" + extensionID + ".xpi"
}
