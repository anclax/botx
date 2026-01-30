package bot

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/pkg/errors"
)

var (
	ErrEmptyMessage     = errors.New("received message without text or callback data")
	ErrInvalidFormInput = errors.New("invalid form input")
	ErrNotFound         = errors.New("not found")
	ErrBadRequest       = errors.New("bad request")
)

// session keys

const (
	SessionKeyRouter     = "__router"
	SessionKeyRouterHist = "__router_history"
	SessionKeyLanguage   = "__language"
)

const (
	CallbackPrefixRoute  = "_route"
	CallbackPrefixSubmit = "_submit"
)

type Button struct {
	ID           string
	Label        string
	CallbackData string
}

type Message struct {
	Text       string
	ParseMode  string
	ButtonGrid [][]Button
}

type Form struct {
	URL    *url.URL
	Idx    int
	Fields []FormField
}

type FormField struct {
	ID        string
	Label     string
	Input     *FormFieldInput
	Validator *string
}

type FormFieldInput struct {
	Schema *FormSchema
	Tip    string
	Value  string
}

type FormSchema struct {
	Type   string
	Format string
}

type FormValues map[string]string

type ValidateResult struct {
	Valid        bool
	ErrorMessage string
}

// BotConnector is an abstraction of the real bot implementation.
// It is used internally by the bot runtime.
type BotConnector interface {
	SendMessage(ctx context.Context, chatID int64, messages *Message) error

	SendForm(ctx context.Context, chatID int64, form *Form) error

	SendCallbackData(ctx context.Context, chatID int64, data string) error

	RegisterBotxHandler(handler BotxHandler)
}

// Bot is a user-facing wrapper around BotConnector.
type Bot struct {
	connector BotConnector
}

func NewBot(connector BotConnector) *Bot {
	return &Bot{connector: connector}
}

func (b *Bot) SendMessage(ctx context.Context, chatID int64, messages *Message) error {
	return b.connector.SendMessage(ctx, chatID, messages)
}

func (b *Bot) SendForm(ctx context.Context, chatID int64, form *Form) error {
	return b.connector.SendForm(ctx, chatID, form)
}

func (b *Bot) SendCallbackData(ctx context.Context, chatID int64, data string) error {
	return b.connector.SendCallbackData(ctx, chatID, data)
}

func (b *Bot) Route(ctx context.Context, chatID int64, url string) error {
	return b.SendCallbackData(ctx, chatID, RouteCallbackData(url))
}

// BotxHandler is the interface of the generated code
type BotxHandler interface {
	HandleTextMessage(ctx context.Context, data string, chatID int64, bot BotConnector) error

	HandleCallbackData(ctx context.Context, data string, chatID int64, bot BotConnector) error

	HandleError(ctx context.Context, err error, chatID int64, bot BotConnector) error

	Validate(ctx context.Context, chatID int64, url *url.URL, validator string, input string) (*ValidateResult, error)
}

func RouteCallbackData(url string) string {
	return fmt.Sprintf("%s:%s", CallbackPrefixRoute, url)
}

func CallbackData(value string) string {
	if value == "" {
		return value
	}
	if strings.HasPrefix(value, "route:") {
		return RouteCallbackData(strings.TrimPrefix(value, "route:"))
	}
	if strings.HasPrefix(value, CallbackPrefixRoute+":") || strings.HasPrefix(value, CallbackPrefixSubmit+":") {
		return value
	}
	if strings.HasPrefix(value, "lang:") {
		return value
	}
	return RouteCallbackData(value)
}

func SubmitForm(url string) string {
	return fmt.Sprintf("%s:%s", CallbackPrefixSubmit, url)
}

type languageContextKey struct{}

func WithLanguage(ctx context.Context, language string) context.Context {
	if ctx == nil || language == "" {
		return ctx
	}
	return context.WithValue(ctx, languageContextKey{}, language)
}

func LanguageFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value := ctx.Value(languageContextKey{})
	if language, ok := value.(string); ok {
		return language
	}
	return ""
}
