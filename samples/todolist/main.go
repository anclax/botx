package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anclax/botx/pkg/core/bot"
	"github.com/anclax/botx/pkg/core/session"
	"go.uber.org/zap"
)

type sampleHandler struct{}

func (h *sampleHandler) HandleTextMessage(ctx context.Context, data string, chatID int64, b *bot.Bot) error {
	if strings.HasPrefix(data, "/") {
		return b.SendMessage(ctx, chatID, &bot.Message{Text: "Use /start to begin or tap buttons."})
	}
	return b.SendMessage(ctx, chatID, &bot.Message{Text: "Use /start to begin."})
}

func (h *sampleHandler) HandleCallbackData(ctx context.Context, _ string, chatID int64, b *bot.Bot) error {
	return b.SendMessage(ctx, chatID, &bot.Message{Text: "Unknown action."})
}

func (h *sampleHandler) HandleError(_ context.Context, _ error, _ int64, _ *bot.Bot) error {
	return nil
}

type sampleCommandHandler struct{}

func (h *sampleCommandHandler) HandleCommandStart(ctx context.Context, chatID int64, b *bot.Bot) error {
	return b.Route(ctx, chatID, "/")
}

func main() {
	token := "8271327448:AAGzW0yNTAI9Gye64h6ezimnLS7bScJYi4E"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger, err := zap.NewDevelopment()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logger.Sync()

	sm, err := session.NewMemorySessionManager()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	backend, err := bot.NewTelegramBot(token, sm, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	telegramBot, ok := backend.(*bot.TelegramBot)
	if !ok {
		fmt.Fprintln(os.Stderr, "telegram backend type assertion failed")
		os.Exit(1)
	}

	store := NewTodoStore()
	stateProvider := NewTodoStateProvider(store)
	formValidator := &TodoFormValidator{}
	defaultHandler := &sampleHandler{}
	commandHandler := &sampleCommandHandler{}

	Register(backend, sm, stateProvider, formValidator, defaultHandler, commandHandler)

	logger.Info("todolist telegram bot started")
	telegramBot.Start(ctx)
}
