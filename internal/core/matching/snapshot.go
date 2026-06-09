package matching

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"exchange/internal/core/order"
)

const BookStateSnapshotVersion = 1

var (
	ErrSnapshotCorrupt         = errors.New("matcher snapshot is corrupt")
	ErrSnapshotRetentionUnsafe = errors.New("matcher snapshot retention is unsafe")
)

type BookStateSnapshot struct {
	SchemaVersion       int           `json:"schema_version"`
	Market              string        `json:"market"`
	BaseAsset           string        `json:"base_asset"`
	QuoteAsset          string        `json:"quote_asset"`
	LastAppliedSequence uint64        `json:"last_applied_sequence"`
	CreatedAt           time.Time     `json:"created_at"`
	ActiveOrders        []order.Order `json:"active_orders"`
	Checksum            string        `json:"checksum"`
}

type SnapshotRetentionGuard struct {
	LatestSnapshotSequence uint64
	OldestRetainedSequence uint64
	SnapshotCreatedAt      time.Time
	Now                    time.Time
	EventRetention         time.Duration
	MaxSnapshotAge         time.Duration
}

func (b *MarketBook) CaptureState(lastAppliedSequence uint64, now time.Time) BookStateSnapshot {
	if b == nil {
		return BookStateSnapshot{SchemaVersion: BookStateSnapshotVersion, CreatedAt: now}
	}
	items := make([]order.Order, 0, len(b.orders))
	for _, price := range b.bids.prices {
		items = append(items, cloneOrders(b.bids.levels[price])...)
	}
	for _, price := range b.asks.prices {
		items = append(items, cloneOrders(b.asks.levels[price])...)
	}
	snapshot := BookStateSnapshot{
		SchemaVersion:       BookStateSnapshotVersion,
		Market:              b.Market,
		BaseAsset:           b.BaseAsset,
		QuoteAsset:          b.QuoteAsset,
		LastAppliedSequence: lastAppliedSequence,
		CreatedAt:           now,
		ActiveOrders:        items,
	}
	_ = snapshot.Seal()
	return snapshot
}

func RestoreMarketBook(snapshot BookStateSnapshot) (*MarketBook, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	book := NewMarketBook(snapshot.Market, snapshot.BaseAsset, snapshot.QuoteAsset)
	if err := book.Load(snapshot.ActiveOrders); err != nil {
		return nil, err
	}
	return book, nil
}

func EncodeBookStateSnapshot(snapshot BookStateSnapshot) ([]byte, error) {
	if snapshot.Checksum == "" {
		if err := snapshot.Seal(); err != nil {
			return nil, err
		}
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(snapshot)
}

func DecodeBookStateSnapshot(payload []byte) (BookStateSnapshot, error) {
	var snapshot BookStateSnapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return BookStateSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return BookStateSnapshot{}, err
	}
	return snapshot, nil
}

func (s *BookStateSnapshot) Seal() error {
	checksum, err := s.computeChecksum()
	if err != nil {
		return err
	}
	s.Checksum = checksum
	return nil
}

func (s BookStateSnapshot) Validate() error {
	if s.SchemaVersion != BookStateSnapshotVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrSnapshotCorrupt, s.SchemaVersion)
	}
	if s.Market == "" {
		return fmt.Errorf("%w: market is required", ErrSnapshotCorrupt)
	}
	if s.Checksum == "" {
		return fmt.Errorf("%w: checksum is required", ErrSnapshotCorrupt)
	}
	expected, err := s.computeChecksum()
	if err != nil {
		return err
	}
	if s.Checksum != expected {
		return fmt.Errorf("%w: checksum mismatch", ErrSnapshotCorrupt)
	}
	return nil
}

func CheckSnapshotRetention(guard SnapshotRetentionGuard) error {
	now := guard.Now
	if now.IsZero() {
		now = time.Now()
	}
	if guard.OldestRetainedSequence > guard.LatestSnapshotSequence+1 {
		return fmt.Errorf("%w: event log starts at %d but snapshot sequence is %d", ErrSnapshotRetentionUnsafe, guard.OldestRetainedSequence, guard.LatestSnapshotSequence)
	}
	if guard.MaxSnapshotAge > 0 {
		age := now.Sub(guard.SnapshotCreatedAt)
		if guard.SnapshotCreatedAt.IsZero() || age > guard.MaxSnapshotAge {
			return fmt.Errorf("%w: snapshot age %s exceeds max age %s", ErrSnapshotRetentionUnsafe, age, guard.MaxSnapshotAge)
		}
	}
	if guard.EventRetention > 0 && guard.MaxSnapshotAge > 0 && guard.EventRetention < guard.MaxSnapshotAge*3 {
		return fmt.Errorf("%w: event retention %s is below 3x max snapshot age %s", ErrSnapshotRetentionUnsafe, guard.EventRetention, guard.MaxSnapshotAge)
	}
	if guard.EventRetention > 0 && !guard.SnapshotCreatedAt.IsZero() {
		age := now.Sub(guard.SnapshotCreatedAt)
		if age > guard.EventRetention/3 {
			return fmt.Errorf("%w: snapshot age %s is too close to event retention %s", ErrSnapshotRetentionUnsafe, age, guard.EventRetention)
		}
	}
	return nil
}

func (s BookStateSnapshot) computeChecksum() (string, error) {
	copy := s
	copy.Checksum = ""
	payload, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func cloneOrders(items []order.Order) []order.Order {
	out := make([]order.Order, len(items))
	copy(out, items)
	return out
}
