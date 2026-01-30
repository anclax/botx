package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/anclax/botx/pkg/core/bot"
	"github.com/pkg/errors"
)

type Frontend struct {
	reader      *bufio.Reader
	writer      *bufio.Writer
	lastButtons []bot.Button
}

func New(reader io.Reader, writer io.Writer) *Frontend {
	return &Frontend{
		reader: bufio.NewReader(reader),
		writer: bufio.NewWriter(writer),
	}
}

func (f *Frontend) SendMessage(ctx context.Context, _ int64, message *bot.Message) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if f.writer == nil {
		return errors.New("cli writer is not configured")
	}
	if message == nil {
		return errors.New("message is required")
	}

	if message.Text != "" {
		if _, err := fmt.Fprintln(f.writer, message.Text); err != nil {
			return errors.Wrap(err, "failed to write message")
		}
	}

	buttons := flattenButtons(message.ButtonGrid)
	if len(buttons) > 0 {
		if message.Text != "" {
			if _, err := fmt.Fprintln(f.writer); err != nil {
				return errors.Wrap(err, "failed to write button spacing")
			}
		}
		for i, btn := range buttons {
			if _, err := fmt.Fprintf(f.writer, "%d) %s -> %s\n", i+1, btn.Label, btn.CallbackData); err != nil {
				return errors.Wrap(err, "failed to write button")
			}
		}
		if _, err := fmt.Fprintln(f.writer); err != nil {
			return errors.Wrap(err, "failed to finish button output")
		}
		f.lastButtons = buttons
	} else {
		f.lastButtons = nil
	}

	if err := f.writer.Flush(); err != nil {
		return errors.Wrap(err, "failed to flush output")
	}
	return nil
}

func (f *Frontend) ReadUpdate(ctx context.Context, chatID int64) (*bot.CLIUpdate, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.reader == nil {
		return nil, errors.New("cli reader is not configured")
	}
	if f.writer == nil {
		return nil, errors.New("cli writer is not configured")
	}

	if _, err := fmt.Fprint(f.writer, "> "); err != nil {
		return nil, errors.Wrap(err, "failed to write prompt")
	}
	if err := f.writer.Flush(); err != nil {
		return nil, errors.Wrap(err, "failed to flush prompt")
	}

	line, err := f.reader.ReadString('\n')
	if err != nil {
		return nil, errors.Wrap(err, "failed to read input")
	}
	input := strings.TrimSpace(line)
	if input == "" {
		return nil, bot.ErrEmptyMessage
	}

	if idx, err := strconv.Atoi(input); err == nil {
		if idx >= 1 && idx <= len(f.lastButtons) {
			return &bot.CLIUpdate{
				ChatID:       chatID,
				CallbackData: f.lastButtons[idx-1].CallbackData,
			}, nil
		}
	}

	if callback := f.resolveCallback(input); callback != "" {
		return &bot.CLIUpdate{
			ChatID:       chatID,
			CallbackData: callback,
		}, nil
	}

	return &bot.CLIUpdate{
		ChatID: chatID,
		Text:   input,
	}, nil
}

func (f *Frontend) resolveCallback(input string) string {
	if strings.HasPrefix(input, bot.CallbackPrefixRoute+":") || strings.HasPrefix(input, bot.CallbackPrefixSubmit+":") {
		return input
	}
	if strings.HasPrefix(input, "route:") {
		return bot.CallbackData(input)
	}
	if strings.HasPrefix(input, "lang:") {
		return input
	}
	for _, btn := range f.lastButtons {
		if input == btn.CallbackData {
			return btn.CallbackData
		}
	}
	return ""
}

func flattenButtons(grid [][]bot.Button) []bot.Button {
	buttons := make([]bot.Button, 0, len(grid))
	for _, row := range grid {
		buttons = append(buttons, row...)
	}
	return buttons
}
