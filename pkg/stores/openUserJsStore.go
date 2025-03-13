package stores

type OpenUserJSStore struct {
	BaseStore
}

func (s *OpenUserJSStore) GetName() string {
	return getName(s.WebstoreURL, `/install/[^/]+/([^.]+).user.js`)
}
