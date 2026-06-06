package assets

import (
	"exchange/internal/core/venue"

	"github.com/ethereum/go-ethereum/common"
)

func Addresses(assetIDs []venue.AssetID) []common.Address {
	seen := make(map[common.Address]struct{}, len(assetIDs))
	out := make([]common.Address, 0, len(assetIDs))
	for _, id := range assetIDs {
		if !common.IsHexAddress(string(id)) {
			continue
		}
		addr := common.HexToAddress(string(id))
		if addr == (common.Address{}) {
			continue
		}
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out
}
