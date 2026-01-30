package vault

type Store interface {
	Get(key string) (any, error)

	Set(key string, value any) error

	Delete(key string) error
}
