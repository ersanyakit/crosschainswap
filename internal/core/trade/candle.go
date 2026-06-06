package trade

import "time"

type Candle struct {
	Market      string    `json:"market"`
	Interval    string    `json:"interval"`
	OpenTime    time.Time `json:"open_time"`
	CloseTime   time.Time `json:"close_time"`
	Open        string    `json:"open"`
	High        string    `json:"high"`
	Low         string    `json:"low"`
	Close       string    `json:"close"`
	VolumeBase  string    `json:"volume_base"`
	VolumeQuote string    `json:"volume_quote"`
	TradeCount  int64     `json:"trade_count"`
	LastTradeAt time.Time `json:"last_trade_at"`
}

type Interval struct {
	Key      string
	Duration time.Duration
}

var SupportedIntervals = []Interval{
	{Key: "1m", Duration: time.Minute},
	{Key: "5m", Duration: 5 * time.Minute},
	{Key: "15m", Duration: 15 * time.Minute},
	{Key: "1h", Duration: time.Hour},
	{Key: "4h", Duration: 4 * time.Hour},
	{Key: "1d", Duration: 24 * time.Hour},
}

func IntervalByKey(key string) (Interval, bool) {
	for _, interval := range SupportedIntervals {
		if interval.Key == key {
			return interval, true
		}
	}
	return Interval{}, false
}
