package stores

type GithubStore struct {
	BaseStore
}

func (s *GithubStore) GetName() string {
	return getName(s.WebstoreURL, `/([^/]+)/`)
}
