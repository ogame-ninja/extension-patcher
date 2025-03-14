package providers

func NewGithubProvider(uri string) *GithubProvider {
	return &GithubProvider{BaseProvider: BaseProvider{uri: uri}}
}

type GithubProvider struct {
	BaseProvider
}

func (s *GithubProvider) GetName() string {
	return getName(s.uri, `/([^/]+)/`)
}
