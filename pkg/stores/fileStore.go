package stores

func NewFileStore(webstoreURL string) *FileStore {
	return &FileStore{BaseStore: BaseStore{WebstoreURL: webstoreURL}}
}

type FileStore struct {
	BaseStore
}
