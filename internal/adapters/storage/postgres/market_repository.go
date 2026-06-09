package postgres

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"exchange/internal/core/decimal"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExchangeMarketStats struct {
	LastPrice string
	Change24h string
	High24h   string
	Low24h    string
	Volume24h string
}

func (r *ExchangeRepository) ListExchangeMarkets(ctx context.Context) ([]ExchangeMarket, error) {
	var models []ExchangeMarket
	err := r.db.WithContext(ctx).
		Clauses(clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "symbol"}}}}).
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (r *ExchangeRepository) RefreshExchangeMarketStats(ctx context.Context, market string) error {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return nil
	}
	stats, err := r.exchangeMarketStats(ctx, market)
	if err != nil {
		return err
	}
	return r.UpdateExchangeMarketStats(ctx, market, stats)
}

func (r *ExchangeRepository) UpdateExchangeMarketStats(ctx context.Context, market string, stats ExchangeMarketStats) error {
	market = strings.ToUpper(strings.TrimSpace(market))
	if market == "" {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&ExchangeMarket{}).
		Where("symbol = ?", market).
		Updates(map[string]any{
			"last_price": statsValue(stats.LastPrice),
			"change_24h": statsValue(stats.Change24h),
			"high_24h":   statsValue(stats.High24h),
			"low_24h":    statsValue(stats.Low24h),
			"volume_24h": statsValue(stats.Volume24h),
			"updated_at": now,
		}).Error
}

func (r *ExchangeRepository) exchangeMarketStats(ctx context.Context, market string) (ExchangeMarketStats, error) {
	candles, err := r.ListCandles(ctx, market, "1m", 1440)
	if err != nil {
		return ExchangeMarketStats{}, err
	}
	if len(candles) == 0 {
		last, err := r.LastTrade(ctx, market)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return zeroExchangeMarketStats(), nil
			}
			return ExchangeMarketStats{}, err
		}
		return ExchangeMarketStats{
			LastPrice: statsValue(last.Price),
			Change24h: "0",
			High24h:   statsValue(last.Price),
			Low24h:    statsValue(last.Price),
			Volume24h: "0",
		}, nil
	}

	first := candles[0]
	last := candles[len(candles)-1]
	high := first.High
	low := first.Low
	volume := "0"
	for _, item := range candles {
		if decimal.Cmp(item.High, high) > 0 {
			high = item.High
		}
		if decimal.Cmp(item.Low, low) < 0 {
			low = item.Low
		}
		volume = decimal.Add(volume, item.VolumeBase)
	}
	return ExchangeMarketStats{
		LastPrice: statsValue(last.Close),
		Change24h: statsPercentChange(first.Open, last.Close),
		High24h:   statsValue(high),
		Low24h:    statsValue(low),
		Volume24h: statsValue(volume),
	}, nil
}

func zeroExchangeMarketStats() ExchangeMarketStats {
	return ExchangeMarketStats{
		LastPrice: "0",
		Change24h: "0",
		High24h:   "0",
		Low24h:    "0",
		Volume24h: "0",
	}
}

func statsValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	return decimal.String(decimal.Parse(value))
}

func statsPercentChange(open string, close string) string {
	openRat := decimal.Parse(open)
	if openRat.Sign() <= 0 {
		return "0"
	}
	diff := new(big.Rat).Sub(decimal.Parse(close), openRat)
	pct := new(big.Rat).Mul(diff, big.NewRat(100, 1))
	pct.Quo(pct, openRat)
	return decimal.String(pct)
}
