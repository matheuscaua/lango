package http

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/kituomenyu/lango/internal/application"
	"github.com/kituomenyu/lango/internal/domain"
	pkgmiddleware "github.com/kituomenyu/lango/pkg/middleware"
)

// MessageHandler exposes the generic outbound send API.
type MessageHandler struct {
	sendUC *application.SendMessageUseCase
}

func NewMessageHandler(sendUC *application.SendMessageUseCase) *MessageHandler {
	return &MessageHandler{sendUC: sendUC}
}

type sendMessageRequest struct {
	Type    string `json:"type"` // "text" | "list" | "buttons"
	Phone   string `json:"phone"`
	Content string `json:"content"`
	// Title, Footer and ButtonText are opaque list-message chrome, forwarded
	// as-is to the provider — lango never generates or interprets them, only
	// some providers (e.g. Twilio's Content API) require ButtonText non-empty.
	Title      string              `json:"title,omitempty"`
	Footer     string              `json:"footer,omitempty"`
	ButtonText string              `json:"button_text,omitempty"`
	Options    []sendMessageOption `json:"options,omitempty"`
}

type sendMessageOption struct {
	ID    string `json:"id"` // opaque to lango — generated and interpreted by the consumer
	Label string `json:"label"`
}

// Send handles POST /v1/integrations/:id/messages
func (h *MessageHandler) Send(c *fiber.Ctx) error {
	consumerID, ok := pkgmiddleware.ConsumerIDFromLocals(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "missing consumer context")
	}

	integrationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid integration id")
	}

	var req sendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Phone == "" {
		return fiber.NewError(fiber.StatusBadRequest, "phone is required")
	}

	msg, err := buildMessage(req)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	err = h.sendUC.Execute(c.Context(), application.SendMessageInput{
		ConsumerID:    consumerID,
		IntegrationID: integrationID,
		Phone:         req.Phone,
		Message:       msg,
		CorrelationID: correlationIDFrom(c),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIntegrationNotOwned), errors.Is(err, domain.ErrIntegrationNotFound):
			return fiber.NewError(fiber.StatusNotFound, "integration not found")
		case errors.Is(err, domain.ErrInvalidPhoneNumber):
			return fiber.NewError(fiber.StatusBadRequest, "phone must be in E.164 format")
		default:
			return fiber.NewError(fiber.StatusBadGateway, "failed to send message")
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func buildMessage(req sendMessageRequest) (*domain.Message, error) {
	switch req.Type {
	case "", "text":
		if req.Content == "" {
			return nil, errors.New("content is required for type=text")
		}
		return &domain.Message{Type: domain.TypeText, Content: req.Content}, nil

	case "list":
		rows := make([]domain.ListRow, 0, len(req.Options))
		for _, opt := range req.Options {
			rows = append(rows, domain.ListRow{RowID: opt.ID, Title: opt.Label})
		}
		list := domain.ListMessage{
			Title:       req.Title,
			Description: req.Content,
			Footer:      req.Footer,
			ButtonText:  req.ButtonText,
			Sections:    []domain.ListSection{{Rows: rows}},
		}
		encoded, err := marshalList(list)
		if err != nil {
			return nil, err
		}
		return &domain.Message{Type: domain.TypeInteractive, Content: encoded}, nil

	default:
		return nil, errors.New("type must be one of: text, list")
	}
}

func marshalList(list domain.ListMessage) (string, error) {
	b, err := json.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("marshal list message: %w", err)
	}
	return string(b), nil
}
