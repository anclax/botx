package bot

import (
	"context"
	"encoding/json"

	"github.com/anclax/botx/pkg/core/session"
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	TgSessionKeyInputState = "__tg_input_state"
)

type TelegramBot struct {
	tgbot *tgbot.Bot
	log   *zap.Logger

	handler BotxHandler

	sm session.SessionManager
}

func (b *TelegramBot) Start(ctx context.Context) {
	b.tgbot.Start(ctx)
}

func NewTelegramBot(token string, sm session.SessionManager, log *zap.Logger) (BotConnector, error) {
	t := &TelegramBot{
		sm:  sm,
		log: log,
	}

	tgbot, err := tgbot.New(token, tgbot.WithDefaultHandler(t.defaultHandler))
	if err != nil {
		return nil, errors.Wrap(err, "failed to create telegram bot")
	}
	t.tgbot = tgbot

	return t, nil
}

func toTgMessage(chatID int64, message *Message) *tgbot.SendMessageParams {
	tgMessage := &tgbot.SendMessageParams{
		ChatID:    chatID,
		Text:      message.Text,
		ParseMode: models.ParseMode(message.ParseMode),
	}

	var inlineKeyboard [][]models.InlineKeyboardButton
	for _, btns := range message.ButtonGrid {
		var row []models.InlineKeyboardButton
		for _, btn := range btns {
			row = append(row, models.InlineKeyboardButton{
				Text:         btn.Label,
				CallbackData: btn.CallbackData,
			})
		}
		inlineKeyboard = append(inlineKeyboard, row)
	}

	if len(inlineKeyboard) > 0 {
		tgMessage.ReplyMarkup = &models.InlineKeyboardMarkup{
			InlineKeyboard: inlineKeyboard,
		}
	}

	return tgMessage
}

func (b *TelegramBot) defaultHandler(ctx context.Context, tgbot *tgbot.Bot, update *models.Update) {
	ctx = updateLanguage(ctx, update)
	chatID, err := fetchChatID(update)
	if err != nil {
		b.handler.HandleError(ctx, err, 0, b)
		return
	}

	if err := b._defaultHandler(ctx, chatID, tgbot, update); err != nil {
		b.handler.HandleError(ctx, err, chatID, b)
		return
	}
}

func (b *TelegramBot) _defaultHandler(ctx context.Context, chatID int64, _ *tgbot.Bot, update *models.Update) error {
	sess, err := b.sm.Get(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to get session")
	}

	// handle callback query
	if update.CallbackQuery != nil {
		data := update.CallbackQuery.Data
		return b.SendCallbackData(ctx, chatID, data)
	}

	if update.Message == nil {
		return errors.Wrapf(ErrEmptyMessage, "udpate: %v", update)
	}

	text := update.Message.Text

	// check if we are in the middle of a form
	val, err := sess.Get(ctx, TgSessionKeyInputState)
	if err == nil {
		form, ok := val.(*Form)
		if !ok {
			return errors.Errorf("invalid input state type: %T", val)
		}
		for {
			validation, err := b.handleInProgressForm(ctx, chatID, sess, text, form)
			if err != nil {
				return errors.Wrap(err, "failed to handle form input")
			}
			if validation.Valid {
				break
			}
			if err := b.SendMessage(ctx, chatID, &Message{
				Text:       validation.ErrorMessage,
				ParseMode:  string(models.ParseModeHTML),
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

	// normal text message
	if err := b.handler.HandleTextMessage(ctx, text, chatID, b); err != nil {
		return errors.Wrap(err, "failed to handle text message")
	}

	return nil
}

func (b *TelegramBot) RegisterBotxHandler(handler BotxHandler) {
	b.handler = handler
}

func (b *TelegramBot) SendMessage(ctx context.Context, chatID int64, message *Message) error {
	_, err := b.tgbot.SendMessage(ctx, toTgMessage(chatID, message))
	return err
}

func (b *TelegramBot) SendCallbackData(ctx context.Context, chatID int64, data string) error {
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

// SendForm sends a form piece by piece as Telegram does not support forms natively. It sends the first
// field and set the state to expect the next text message to be the input for that field. More can be
// found in handleInProgressForm.
func (b *TelegramBot) SendForm(ctx context.Context, chatID int64, form *Form) error {
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
	if err := sess.Set(ctx, TgSessionKeyInputState, form); err != nil {
		return errors.Wrap(err, "failed to set input state in session")
	}
	return nil
}

func (b *TelegramBot) sendFormField(ctx context.Context, chatID int64, field *FormField) error {
	if field.Input != nil {
		input := field.Input
		msg := &Message{
			Text:      input.Tip,
			ParseMode: string(models.ParseModeHTML),
		}
		if err := b.SendMessage(ctx, chatID, msg); err != nil {
			return errors.Wrap(err, "failed to send form field prompt")
		}
		return nil
	} else {
		return errors.Errorf("field has no type, should have `input`: %+v", field)
	}
}

// handleInProgressForm processes the input for the current field in the form. If the input is valid,
// it moves to the next field or submits the form if all fields are filled.
func (b *TelegramBot) handleInProgressForm(ctx context.Context, chatID int64, sess session.Session, text string, form *Form) (*ValidateResult, error) {
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
		// form completed
		if err := sess.Delete(ctx, TgSessionKeyInputState); err != nil {
			return nil, errors.Wrap(err, "failed to clear input state from session")
		}
		raw, err := marshalFormValues(form)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal form values to json")
		}
		query := form.URL.Query()
		query.Add("values", string(raw))
		form.URL.RawQuery = query.Encode()
		// submit form
		if err := b.SendCallbackData(ctx, chatID, SubmitForm(form.URL.String())); err != nil {
			return nil, errors.Wrap(err, "failed to submit form data")
		}
	} else {
		// save to session
		if err := sess.Set(ctx, TgSessionKeyInputState, form); err != nil {
			return nil, errors.Wrap(err, "failed to save input state to session")
		}
		// send next field
		if err := b.sendFormField(ctx, chatID, &form.Fields[form.Idx]); err != nil {
			return nil, errors.Wrap(err, "failed to send next form field")
		}
	}

	return &ValidateResult{Valid: true}, nil
}

func marshalFormValues(form *Form) ([]byte, error) {
	m := make(FormValues)
	for _, field := range form.Fields {
		if field.Input != nil {
			m[field.ID] = field.Input.Value
		}
	}
	return json.Marshal(m)
}

func updateLanguage(ctx context.Context, update *models.Update) context.Context {
	if update == nil {
		return ctx
	}
	language := ""
	if update.Message != nil && update.Message.From != nil {
		language = update.Message.From.LanguageCode
	}
	if language == "" && update.CallbackQuery != nil {
		language = update.CallbackQuery.From.LanguageCode
	}
	if language == "" {
		return ctx
	}
	return WithLanguage(ctx, language)
}

func fetchMessage(update *models.Update) (*models.Message, error) {
	if update.Message != nil {
		return update.Message, nil
	}
	if update.CallbackQuery != nil {
		return update.CallbackQuery.Message.Message, nil
	}
	return nil, errors.Errorf("cannot fetch Message from update: %+v", update)
}

func fetchChatID(update *models.Update) (int64, error) {
	msg, err := fetchMessage(update)
	if err != nil {
		return 0, err
	}
	return msg.Chat.ID, nil
}
