package venue

import "context"

type PoolScanner interface {
	ScanPools(ctx context.Context) ([]Pool, error)
	LoadPool(ctx context.Context, id PoolID) (*Pool, error)
}
