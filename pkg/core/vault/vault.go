package vault

type Vault struct {
	store Store
}

func NewVault(store Store) *Vault {
	return &Vault{
		store: store,
	}
}
