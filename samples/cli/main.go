package main

import (
	"context"
	stdErrors "errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anclax/botx/pkg/core/bot"
	"github.com/anclax/botx/pkg/core/session"
	clifront "github.com/anclax/botx/samples/cli/frontend"
	"github.com/anclax/botx/samples/common"
)

type sampleHandler struct {
	cli *bot.CLIBot
}

func (h *sampleHandler) HandleTextMessage(ctx context.Context, data string, chatID int64, b *bot.Bot) error {
	if strings.HasPrefix(data, "/") {
		return b.Route(ctx, chatID, data)
	}
	return b.SendMessage(ctx, chatID, &bot.Message{Text: "Type /start or select a button."})
}

func (h *sampleHandler) HandleCallbackData(ctx context.Context, _ string, chatID int64, b *bot.Bot) error {
	return b.SendMessage(ctx, chatID, &bot.Message{Text: "Unknown action."})
}

func (h *sampleHandler) HandleError(_ context.Context, _ error, _ int64, _ *bot.Bot) error {
	return nil
}

type sampleCommandHandler struct {
	cli *bot.CLIBot
}

func (h *sampleCommandHandler) HandleCommandStart(ctx context.Context, chatID int64, b *bot.Bot) error {
	return b.Route(ctx, chatID, "/")
}

func main() {
	ctx := context.Background()
	sm, err := session.NewMemorySessionManager()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	frontend := clifront.New(os.Stdin, os.Stdout)
	cliBot, err := bot.NewCLIBot(sm, frontend)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	store := common.NewAddressStore()
	stateProvider := common.NewSampleStateProvider(store)
	formValidator := &common.SampleFormValidator{}
	defaultHandler := &sampleHandler{cli: cliBot}
	commandHandler := &sampleCommandHandler{cli: cliBot}

	common.Register(cliBot, sm, stateProvider, formValidator, defaultHandler, commandHandler)

	_ = cliBot.SendMessage(ctx, bot.DefaultCLIChatID, &bot.Message{Text: "Type /start to begin."})

	for {
		update, err := frontend.ReadUpdate(ctx, bot.DefaultCLIChatID)
		if err != nil {
			if stdErrors.Is(err, io.EOF) {
				break
			}
			if stdErrors.Is(err, bot.ErrEmptyMessage) {
				continue
			}
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if err := cliBot.HandleUpdate(ctx, update); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
}
