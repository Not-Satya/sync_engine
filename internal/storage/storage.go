type storage interface {
	Stat(path string) error
	
	Rename(path string, data []byte) error

	Put(path string, data []byte) error

	Get(path string) ([]byte, error)

	Delete(path string) error

	List() ([]string, error)
}