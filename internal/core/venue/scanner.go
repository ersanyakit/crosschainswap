package venue

import "context"

type PoolBatchHandler func(ctx context.Context, pools []Pool) error

type PoolScanner interface {
	ScanPools(ctx context.Context) ([]Pool, error)
	LoadPool(ctx context.Context, id PoolID) (*Pool, error)
}

type StreamingPoolScanner interface {
	ScanPoolsStream(ctx context.Context, handle PoolBatchHandler) (int, error)
}
