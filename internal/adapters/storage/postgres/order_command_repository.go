package postgres

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	OrderCommandTypeNewOrder = "new_order"

	OrderCommandStatusPending     = "pending"
	OrderCommandStatusCompleted   = "completed"
	OrderCommandStatusRejected    = "rejected"
	OrderCommandStatusQuarantined = "quarantined"
)

type OrderCommand struct {
	ID            string
	ClientOrderID string
	UserID        string
	Market        string
	Type          string
	PayloadHash   string
	Payload       string
	Status        string
	Outcome       string
	OrderID       string
	Attempts      int
	LastError     string
	QuarantinedAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (r *ExchangeRepository) CreateOrderCommand(ctx context.Context, item OrderCommand) error {
	now := time.Now()
	model := ExchangeOrderCommand{
		ID:            strings.TrimSpace(item.ID),
		ClientOrderID: strings.TrimSpace(item.ClientOrderID),
		UserID:        strings.TrimSpace(item.UserID),
		Market:        strings.ToUpper(strings.TrimSpace(item.Market)),
		Type:          strings.TrimSpace(item.Type),
		PayloadHash:   strings.TrimSpace(item.PayloadHash),
		Payload:       strings.TrimSpace(item.Payload),
		Status:        strings.TrimSpace(item.Status),
		Outcome:       strings.TrimSpace(item.Outcome),
		OrderID:       strings.TrimSpace(item.OrderID),
		Attempts:      item.Attempts,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if model.ID == "" || model.UserID == "" || model.ClientOrderID == "" || model.PayloadHash == "" || model.Payload == "" {
		return nil
	}
	if model.Type == "" {
		model.Type = OrderCommandTypeNewOrder
	}
	if model.Status == "" {
		model.Status = OrderCommandStatusPending
	}
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r *ExchangeRepository) GetOrderCommandForUpdate(ctx context.Context, id string) (*OrderCommand, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var model ExchangeOrderCommand
	err := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&model, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	out := modelToOrderCommand(model)
	return &out, nil
}

func (r *ExchangeRepository) GetOrderCommand(ctx context.Context, id string) (*OrderCommand, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var model ExchangeOrderCommand
	err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	out := modelToOrderCommand(model)
	return &out, nil
}

func (r *ExchangeRepository) FindOrderCommandByClientID(ctx context.Context, userID string, clientOrderID string) (*OrderCommand, error) {
	var model ExchangeOrderCommand
	err := r.db.WithContext(ctx).
		Where(&ExchangeOrderCommand{UserID: strings.TrimSpace(userID), ClientOrderID: strings.TrimSpace(clientOrderID)}).
		First(&model).Error
	if err != nil {
		return nil, err
	}
	out := modelToOrderCommand(model)
	return &out, nil
}

func (r *ExchangeRepository) CompleteOrderCommand(ctx context.Context, id string, orderID string, outcome string) error {
	return r.updateOrderCommand(ctx, id, map[string]any{
		"status":     OrderCommandStatusCompleted,
		"outcome":    strings.TrimSpace(outcome),
		"order_id":   strings.TrimSpace(orderID),
		"last_error": "",
	})
}

func (r *ExchangeRepository) RejectOrderCommand(ctx context.Context, id string, message string) error {
	return r.updateOrderCommand(ctx, id, map[string]any{
		"status":     OrderCommandStatusRejected,
		"outcome":    "rejected",
		"last_error": strings.TrimSpace(message),
	})
}

func (r *ExchangeRepository) QuarantineOrderCommand(ctx context.Context, id string, message string) error {
	now := time.Now()
	return r.updateOrderCommand(ctx, id, map[string]any{
		"status":         OrderCommandStatusQuarantined,
		"outcome":        "command_quarantined",
		"last_error":     strings.TrimSpace(message),
		"quarantined_at": now,
	})
}

func (r *ExchangeRepository) updateOrderCommand(ctx context.Context, id string, values map[string]any) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	values["updated_at"] = time.Now()
	return r.db.WithContext(ctx).
		Model(&ExchangeOrderCommand{}).
		Where(&ExchangeOrderCommand{ID: id}).
		Updates(values).Error
}

func modelToOrderCommand(model ExchangeOrderCommand) OrderCommand {
	return OrderCommand{
		ID:            model.ID,
		ClientOrderID: model.ClientOrderID,
		UserID:        model.UserID,
		Market:        model.Market,
		Type:          model.Type,
		PayloadHash:   model.PayloadHash,
		Payload:       model.Payload,
		Status:        model.Status,
		Outcome:       model.Outcome,
		OrderID:       model.OrderID,
		Attempts:      model.Attempts,
		LastError:     model.LastError,
		QuarantinedAt: model.QuarantinedAt,
		CreatedAt:     model.CreatedAt,
		UpdatedAt:     model.UpdatedAt,
	}
}
