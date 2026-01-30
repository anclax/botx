package main

import (
	"bytes"
	"context"
	stdErrors "errors"
	"io"
	"strings"
	"testing"

	"github.com/anclax/botx/pkg/core/bot"
	"github.com/anclax/botx/pkg/core/session"
	clifront "github.com/anclax/botx/samples/cli/frontend"
	"github.com/anclax/botx/samples/common"
)

func TestCLISampleFlow(t *testing.T) {
	ctx := context.Background()
	sm, err := session.NewMemorySessionManager()
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}

	input := strings.NewReader(strings.Join([]string{
		"/start",
		"1",
		"1",
		"T1234 note",
		"/address",
		"1",
		"3",
		"home",
		"1",
		"4",
		"T5678",
		"1",
		"2",
		"1",
		"",
	}, "\n"))
	var output bytes.Buffer
	frontend := clifront.New(input, &output)
	cliBot, err := bot.NewCLIBot(sm, frontend)
	if err != nil {
		t.Fatalf("cli bot: %v", err)
	}

	store := common.NewAddressStore()
	stateProvider := common.NewSampleStateProvider(store)
	formValidator := &common.SampleFormValidator{}
	defaultHandler := &sampleHandler{cli: cliBot}
	commandHandler := &sampleCommandHandler{cli: cliBot}

	common.Register(cliBot, sm, stateProvider, formValidator, defaultHandler, commandHandler)

	for {
		update, err := frontend.ReadUpdate(ctx, bot.DefaultCLIChatID)
		if err != nil {
			if stdErrors.Is(err, io.EOF) {
				break
			}
			if stdErrors.Is(err, bot.ErrEmptyMessage) {
				continue
			}
			t.Fatalf("read update: %v", err)
		}
		if err := cliBot.HandleUpdate(ctx, update); err != nil {
			t.Fatalf("handle update: %v", err)
		}
	}

	log := output.String()
	if !strings.Contains(log, "欢迎使用地址管理机器人") {
		t.Fatalf("expected root page output, got: %s", log)
	}
	if !strings.Contains(log, "暂无地址，请添加地址") {
		t.Fatalf("expected empty list output, got: %s", log)
	}
	if !strings.Contains(log, "地址添加成功") {
		t.Fatalf("expected add success output, got: %s", log)
	}
	if !strings.Contains(log, "地址修改成功") {
		t.Fatalf("expected edit success output, got: %s", log)
	}
	if !strings.Contains(log, "地址删除成功") {
		t.Fatalf("expected delete success output, got: %s", log)
	}
	if !strings.Contains(log, "地址: T5678") {
		t.Fatalf("expected updated address in output, got: %s", log)
	}
	if !strings.Contains(log, "备注: home") {
		t.Fatalf("expected updated name in output, got: %s", log)
	}
}
