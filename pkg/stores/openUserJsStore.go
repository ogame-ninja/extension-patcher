package stores

func NewOpenUserJSStore(webstoreURL string) *OpenUserJSStore {
	return &OpenUserJSStore{BaseStore: BaseStore{WebstoreURL: webstoreURL}}
}

type OpenUserJSStore struct {
	BaseStore
}

func (s *OpenUserJSStore) GetName() string {
	return getName(s.WebstoreURL, `/install/[^/]+/([^.]+).user.js`)
}
