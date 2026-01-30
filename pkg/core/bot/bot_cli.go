package bot

import (
	"context"

	"github.com/cloudcarver/botx/pkg/core/session"
	"github.com/pkg/errors"
)

const (
	CliSessionKeyInputState       = "__cli_input_state"
	DefaultCLIChatID        int64 = 1
)

type CLIUpdate struct {
	ChatID       int64
	Text         string
	CallbackData string
}

type CLIFrontend interface {
	SendMessage(ctx context.Context, chatID int64, message *Message) error
}

type CLIBot struct {
	frontend CLIFrontend
	handler  BotxHandler
	sm       session.SessionManager
}

func NewCLIBot(sm session.SessionManager, frontend CLIFrontend) (*CLIBot, error) {
	if sm == nil {
		return nil, errors.New("session manager is required")
	}
	if frontend == nil {
		return nil, errors.New("cli frontend is required")
	}
	return &CLIBot{
		frontend: frontend,
		sm:       sm,
	}, nil
}

func (b *CLIBot) RegisterBotxHandler(handler BotxHandler) {
	b.handler = handler
}

func (b *CLIBot) HandleUpdate(ctx context.Context, update *CLIUpdate) error {
	if update == nil {
		return errors.New("cli update is required")
	}
	chatID := update.ChatID
	if chatID == 0 {
		chatID = DefaultCLIChatID
	}
	if err := b.handleUpdate(ctx, chatID, update); err != nil {
		if b.handler == nil {
			return err
		}
		if handleErr := b.handler.HandleError(ctx, err, chatID, b); handleErr != nil {
			return handleErr
		}
	}
	return nil
}

func (b *CLIBot) SendMessage(ctx context.Context, chatID int64, message *Message) error {
	if b.frontend == nil {
		return errors.New("cli frontend is not configured")
	}
	return b.frontend.SendMessage(ctx, chatID, message)
}

func (b *CLIBot) SendForm(ctx context.Context, chatID int64, form *Form) error {
	if len(form.Fields) == 0 {
		return errors.New("form has no fields")
	}
	if err := b.sendFormField(ctx, chatID, &form.Fields[0]); err != nil {
		return errors.Wrap(err, "failed to send first form field")
	}
	sess, err := b.sm.Get(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to get session")
	}
	if err := sess.Set(ctx, CliSessionKeyInputState, form); err != nil {
		return errors.Wrap(err, "failed to set input state in session")
	}
	return nil
}

func (b *CLIBot) SendCallbackData(ctx context.Context, chatID int64, data string) error {
	if b.handler == nil {
		return errors.New("botx handler is not registered")
	}
	if data == "" {
		return errors.New("callback data is required")
	}
	if err := b.handler.HandleCallbackData(ctx, data, chatID, b); err != nil {
		return errors.Wrap(err, "failed to handle callback data")
	}
	return nil
}

func (b *CLIBot) handleUpdate(ctx context.Context, chatID int64, update *CLIUpdate) error {
	if b.handler == nil {
		return errors.New("botx handler is not registered")
	}
	sess, err := b.sm.Get(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to get session")
	}

	if update.CallbackData != "" {
		return b.SendCallbackData(ctx, chatID, update.CallbackData)
	}

	if update.Text == "" {
		return errors.Wrap(ErrEmptyMessage, "cli update has no text or callback data")
	}

	val, err := sess.Get(ctx, CliSessionKeyInputState)
	if err == nil {
		form, ok := val.(*Form)
		if !ok {
			return errors.Errorf("invalid input state type: %T", val)
		}
		for {
			validation, err := b.handleInProgressForm(ctx, chatID, sess, update.Text, form)
			if err != nil {
				return errors.Wrap(err, "failed to handle form input")
			}
			if validation.Valid {
				break
			}
			if err := b.SendMessage(ctx, chatID, &Message{
				Text:       validation.ErrorMessage,
				ParseMode:  "HTML",
				ButtonGrid: [][]Button{},
			}); err != nil {
				return errors.Wrap(err, "failed to send validation error message")
			}
		}
		return nil
	}
	if !errors.Is(err, session.ErrKeyNotFound) {
		return errors.Wrap(err, "failed to get input state from session")
	}

	if err := b.handler.HandleTextMessage(ctx, update.Text, chatID, b); err != nil {
		return errors.Wrap(err, "failed to handle text message")
	}
	return nil
}

func (b *CLIBot) sendFormField(ctx context.Context, chatID int64, field *FormField) error {
	if field.Input == nil {
		return errors.Errorf("field has no type, should have `input`: %+v", field)
	}
	msg := &Message{
		Text:      field.Input.Tip,
		ParseMode: "HTML",
	}
	if err := b.SendMessage(ctx, chatID, msg); err != nil {
		return errors.Wrap(err, "failed to send form field prompt")
	}
	return nil
}

func (b *CLIBot) handleInProgressForm(ctx context.Context, chatID int64, sess session.Session, text string, form *Form) (*ValidateResult, error) {
	field := form.Fields[form.Idx]
	if field.Validator != nil {
		result, err := b.handler.Validate(ctx, chatID, form.URL, *field.Validator, text)
		if err != nil {
			return nil, errors.Wrap(err, "failed to validate form input")
		}
		if !result.Valid {
			return result, nil
		}
	}
	if field.Input != nil {
		field.Input.Value = text
	}
	form.Fields[form.Idx] = field
	form.Idx++
	if form.Idx == len(form.Fields) {
		if err := sess.Delete(ctx, CliSessionKeyInputState); err != nil {
			return nil, errors.Wrap(err, "failed to clear input state from session")
		}
		raw, err := marshalFormValues(form)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal form values to json")
		}
		query := form.URL.Query()
		query.Add("values", string(raw))
		form.URL.RawQuery = query.Encode()
		if err := b.SendCallbackData(ctx, chatID, SubmitForm(form.URL.String())); err != nil {
			return nil, errors.Wrap(err, "failed to submit form data")
		}
	} else {
		if err := sess.Set(ctx, CliSessionKeyInputState, form); err != nil {
			return nil, errors.Wrap(err, "failed to save input state to session")
		}
		if err := b.sendFormField(ctx, chatID, &form.Fields[form.Idx]); err != nil {
			return nil, errors.Wrap(err, "failed to send next form field")
		}
	}

	return &ValidateResult{Valid: true}, nil
}
