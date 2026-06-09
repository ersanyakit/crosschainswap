package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"exchange/internal/core/balance"
	"exchange/internal/core/decimal"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ReservationStatusActive   = "active"
	ReservationStatusConsumed = "consumed"
	ReservationStatusReleased = "released"
	ReservationStatusClosed   = "closed"
)

type Reservation struct {
	ID              string
	UserID          string
	Market          string
	Asset           string
	Amount          string
	ConsumedAmount  string
	ReleasedAmount  string
	RemainingAmount string
	Status          string
	OrderID         string
	CommandID       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (r *ExchangeRepository) GetReservation(ctx context.Context, id string) (*Reservation, error) {
	var model ExchangeReservation
	err := r.db.WithContext(ctx).First(&model, "id = ?", strings.TrimSpace(id)).Error
	if err != nil {
		return nil, err
	}
	out := modelToReservation(model)
	return &out, nil
}

func (r *ExchangeRepository) createReservation(ctx context.Context, item Reservation) error {
	model := ExchangeReservation{
		ID:              strings.TrimSpace(item.ID),
		UserID:          strings.TrimSpace(item.UserID),
		Market:          strings.ToUpper(strings.TrimSpace(item.Market)),
		Asset:           strings.ToUpper(strings.TrimSpace(item.Asset)),
		Amount:          item.Amount,
		ConsumedAmount:  zeroIfEmpty(item.ConsumedAmount),
		ReleasedAmount:  zeroIfEmpty(item.ReleasedAmount),
		RemainingAmount: item.RemainingAmount,
		Status:          strings.TrimSpace(item.Status),
		OrderID:         strings.TrimSpace(item.OrderID),
		CommandID:       strings.TrimSpace(item.CommandID),
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}
	if model.ID == "" || model.UserID == "" || model.Market == "" || model.Asset == "" || model.OrderID == "" {
		return fmt.Errorf("reservation requires id, user_id, market, asset and order_id")
	}
	if decimal.Cmp(model.Amount, "0") <= 0 || decimal.Cmp(model.RemainingAmount, "0") <= 0 {
		return fmt.Errorf("%w: reservation amount must be positive", balance.ErrInsufficientBalance)
	}
	if model.Status == "" {
		model.Status = ReservationStatusActive
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now()
	}
	if model.UpdatedAt.IsZero() {
		model.UpdatedAt = model.CreatedAt
	}
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r *ExchangeRepository) consumeReservation(ctx context.Context, id string, amount string) error {
	return r.applyReservationDelta(ctx, id, amount, true)
}

func (r *ExchangeRepository) releaseReservation(ctx context.Context, id string, amount string) error {
	return r.applyReservationDelta(ctx, id, amount, false)
}

func (r *ExchangeRepository) applyReservationDelta(ctx context.Context, id string, amount string, consume bool) error {
	id = strings.TrimSpace(id)
	if id == "" || decimal.Cmp(amount, "0") <= 0 {
		return nil
	}

	var model ExchangeReservation
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&model, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if decimal.Cmp(model.RemainingAmount, amount) < 0 {
		return balance.ErrInsufficientBalance
	}

	if consume {
		model.ConsumedAmount = decimal.Add(model.ConsumedAmount, amount)
	} else {
		model.ReleasedAmount = decimal.Add(model.ReleasedAmount, amount)
	}
	model.RemainingAmount = decimal.SubFloorZero(model.RemainingAmount, amount)
	model.Status = reservationStatus(model)
	model.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(&model).Error
}

func reservationStatus(model ExchangeReservation) string {
	if decimal.Cmp(model.RemainingAmount, "0") > 0 {
		return ReservationStatusActive
	}
	if decimal.Cmp(model.ConsumedAmount, "0") > 0 && decimal.Cmp(model.ReleasedAmount, "0") > 0 {
		return ReservationStatusClosed
	}
	if decimal.Cmp(model.ConsumedAmount, "0") > 0 {
		return ReservationStatusConsumed
	}
	return ReservationStatusReleased
}

func modelToReservation(model ExchangeReservation) Reservation {
	return Reservation{
		ID:              model.ID,
		UserID:          model.UserID,
		Market:          model.Market,
		Asset:           model.Asset,
		Amount:          zeroIfEmpty(model.Amount),
		ConsumedAmount:  zeroIfEmpty(model.ConsumedAmount),
		ReleasedAmount:  zeroIfEmpty(model.ReleasedAmount),
		RemainingAmount: zeroIfEmpty(model.RemainingAmount),
		Status:          model.Status,
		OrderID:         model.OrderID,
		CommandID:       model.CommandID,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}
