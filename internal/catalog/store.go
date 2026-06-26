package catalog

import "context"

type Store interface {
	Close() error
	Lookup(ctx context.Context, barcode string) (Product, error)
	Import(ctx context.Context, products <-chan Product, parseErrors <-chan error) (int64, error)
}
