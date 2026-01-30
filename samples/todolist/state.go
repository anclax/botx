package main

import (
	"context"
	"net/url"
	"strings"

	"github.com/cloudcarver/botx/pkg/core/bot"
)

type TodoStateProvider struct {
	store *TodoStore
}

func NewTodoStateProvider(store *TodoStore) *TodoStateProvider {
	return &TodoStateProvider{store: store}
}

func (p *TodoStateProvider) ProvideRootState(ctx context.Context, chatID int64, parameters *ParametersPageRoot) (*StatePageRoot, error) {
	items := p.store.List()
	return NewStatePageRoot(items, len(items)), nil
}

func (p *TodoStateProvider) ProvideI18nState(ctx context.Context, chatID int64, parameters *ParametersPageI18n) (*StatePageI18n, error) {
	return &StatePageI18n{}, nil
}

func (p *TodoStateProvider) ProvideTodoAddState(ctx context.Context, chatID int64, form *FormTodoAdd, parameters *ParametersPageTodoAdd) (*StatePageTodoAdd, error) {
	if form == nil {
		return NewStatePageTodoAdd(false, "missing form"), nil
	}
	title := strings.TrimSpace(form.GetTitle())
	if title == "" {
		return NewStatePageTodoAdd(false, "title is required"), nil
	}
	_ = p.store.Add(title)
	return NewStatePageTodoAdd(true, ""), nil
}

func (p *TodoStateProvider) ProvideTodoIDState(ctx context.Context, chatID int64, parameters *ParametersPageTodoID) (*StatePageTodoID, error) {
	if parameters == nil {
		return NewStatePageTodoID(Todo{}), nil
	}
	item, err := p.store.Get(parameters.GetID())
	if err != nil {
		return NewStatePageTodoID(Todo{}), err
	}
	return NewStatePageTodoID(item), nil
}

func (p *TodoStateProvider) ProvideTodoToggleState(ctx context.Context, chatID int64, parameters *ParametersPageTodoToggle) (*StatePageTodoToggle, error) {
	if parameters == nil {
		return NewStatePageTodoToggle(false, "missing id", false), nil
	}
	item, err := p.store.Toggle(parameters.GetID())
	if err != nil {
		return NewStatePageTodoToggle(false, err.Error(), false), nil
	}
	return NewStatePageTodoToggle(true, "", item.GetDone()), nil
}

func (p *TodoStateProvider) ProvideTodoDeleteState(ctx context.Context, chatID int64, parameters *ParametersPageTodoDelete) (*StatePageTodoDelete, error) {
	if parameters == nil {
		return NewStatePageTodoDelete(false, "missing id"), nil
	}
	if err := p.store.Delete(parameters.GetID()); err != nil {
		return NewStatePageTodoDelete(false, err.Error()), nil
	}
	return NewStatePageTodoDelete(true, ""), nil
}

type TodoFormValidator struct{}

func (v *TodoFormValidator) ValidateFormTodoAdd(ctx context.Context, chatID int64, url *url.URL, input string) (*bot.ValidateResult, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return &bot.ValidateResult{Valid: false, ErrorMessage: "title is required"}, nil
	}
	if len(value) > 200 {
		return &bot.ValidateResult{Valid: false, ErrorMessage: "title too long"}, nil
	}
	return &bot.ValidateResult{Valid: true}, nil
}
