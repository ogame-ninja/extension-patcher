package stores

func NewGithubStore(webstoreURL string) *GithubStore {
	return &GithubStore{BaseStore: BaseStore{WebstoreURL: webstoreURL}}
}

type GithubStore struct {
	BaseStore
}

func (s *GithubStore) GetName() string {
	return getName(s.WebstoreURL, `/([^/]+)/`)
}
