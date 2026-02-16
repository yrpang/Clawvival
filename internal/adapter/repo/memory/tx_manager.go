package memory

import "context"

type TxManager struct {
	store *Store
}

func NewTxManager(store *Store) TxManager {
	return TxManager{store: store}
}

func (t TxManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.store.mu.Lock()
	defer t.store.mu.Unlock()
	return fn(ctx)
}
