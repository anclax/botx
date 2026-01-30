package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/cloudcarver/botx/pkg/core/bot"
	"github.com/cloudcarver/botx/pkg/core/routepath"
	"github.com/cloudcarver/botx/pkg/core/session"
	"github.com/pkg/errors"
)

// schemas

type Todo struct {
	ID    int64
	title string
	done  bool
}

func NewTodo(ID int64, title string, done bool) *Todo {
	return &Todo{
		ID:    ID,
		title: title,
		done:  done,
	}
}

func (v Todo) GetTitle() string {
	return v.title
}

func (v Todo) GetDone() bool {
	return v.done
}

// Core architecture components

type BotxHandler struct {
	renderer       *PageRenderer
	bot            *bot.Bot
	sm             session.SessionManager
	sp             StateProvider
	formValidator  FormValidator
	commandHandler CommandHandler
	defaultHandler Handler
}

type Handler interface {
	HandleTextMessage(ctx context.Context, data string, chatID int64, b *bot.Bot) error

	HandleCallbackData(ctx context.Context, data string, chatID int64, b *bot.Bot) error

	HandleError(ctx context.Context, err error, chatID int64, b *bot.Bot) error
}

type CommandHandler interface {
	HandleCommandStart(ctx context.Context, chatID int64, b *bot.Bot) error
}

// Register bot handler to bot. the param bot and param stateProvider is implemented by user
func Register(connector bot.BotConnector, sm session.SessionManager, stateProvider StateProvider, formValidator FormValidator, handler Handler, commandHandler CommandHandler) {
	wrapped := bot.NewBot(connector)
	botxHandler := &BotxHandler{
		renderer:       &PageRenderer{wrapped},
		bot:            wrapped,
		sm:             sm,
		sp:             stateProvider,
		formValidator:  formValidator,
		commandHandler: commandHandler,
		defaultHandler: handler,
	}

	connector.RegisterBotxHandler(botxHandler)
}

func (h *BotxHandler) getRouter(ctx context.Context, chatID int64) (*bot.Router, error) {
	sess, err := h.sm.Get(ctx, chatID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get session")
	}

	router, err := sess.Get(ctx, bot.SessionKeyRouter)
	if err != nil {
		if !errors.Is(err, session.ErrKeyNotFound) {
			return nil, errors.Wrap(err, "failed to get router")
		}
		router, err = bot.CreateRouter(ctx, chatID, sess)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create router")
		}
	}
	return router.(*bot.Router), nil
}

func (h *BotxHandler) withLanguage(ctx context.Context, chatID int64) (context.Context, error) {
	if h.sm == nil {
		return ctx, nil
	}
	lang := ""
	sess, err := h.sm.Get(ctx, chatID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get session")
	}
	value, err := sess.Get(ctx, bot.SessionKeyLanguage)
	if err == nil {
		if text, ok := value.(string); ok {
			lang = strings.TrimSpace(text)
		}
	} else if !errors.Is(err, session.ErrKeyNotFound) {
		return nil, errors.Wrap(err, "failed to get language from session")
	}
	if lang == "" {
		lang = bot.LanguageFromContext(ctx)
	}
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return ctx, nil
	}
	return bot.WithLanguage(ctx, lang), nil
}

func (h *BotxHandler) handleRoute(ctx context.Context, chatID int64, data string) error {
	langCtx, err := h.withLanguage(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to resolve language")
	}
	ctx = langCtx

	router, err := h.getRouter(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to get router")
	}

	routeURL := strings.TrimPrefix(data, fmt.Sprintf("%s:", bot.CallbackPrefixRoute))

	if routeURL == "back" {
		last, err := router.Back(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to go back")
		}
		routeURL = last
	} else {
		if err := router.Push(ctx, routeURL); err != nil {
			return errors.Wrap(err, "failed to push route")
		}
	}

	url, err := url.Parse(routeURL)
	if err != nil {
		return errors.Wrap(err, "failed to parse route uri")
	}
	if err := h.onRoute(ctx, chatID, url); err != nil {
		return errors.Wrap(err, "failed to render page")
	}
	return nil
}

func (h *BotxHandler) handleSubmit(ctx context.Context, chatID int64, data string) error {
	langCtx, err := h.withLanguage(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to resolve language")
	}
	ctx = langCtx

	submitURL := strings.TrimPrefix(data, fmt.Sprintf("%s:", bot.CallbackPrefixSubmit))

	url, err := url.Parse(submitURL)
	if err != nil {
		return errors.Wrap(err, "failed to parse route uri")
	}
	if err := h.onSubmit(ctx, chatID, url); err != nil {
		return errors.Wrap(err, "failed to render page")
	}
	return nil
}

func (h *BotxHandler) handleLanguage(ctx context.Context, chatID int64, data string) error {
	lang := strings.TrimSpace(strings.TrimPrefix(data, "lang:"))
	if lang == "" {
		return errors.Wrap(bot.ErrBadRequest, "missing language")
	}
	lang = strings.ToLower(lang)

	sess, err := h.sm.Get(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to get session")
	}
	if err := sess.Set(ctx, bot.SessionKeyLanguage, lang); err != nil {
		return errors.Wrap(err, "failed to set language")
	}
	ctx = bot.WithLanguage(ctx, lang)

	router, err := h.getRouter(ctx, chatID)
	if err != nil {
		return errors.Wrap(err, "failed to get router")
	}
	hist, err := router.History(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get router history")
	}
	current := "/"
	if len(hist) != 0 {
		current = hist[len(hist)-1]
	}
	url, err := url.Parse(current)
	if err != nil {
		return errors.Wrap(err, "failed to parse route uri")
	}
	if err := h.onRoute(ctx, chatID, url); err != nil {
		return errors.Wrap(err, "failed to render page")
	}
	return nil
}

func (h *BotxHandler) HandleTextMessage(ctx context.Context, data string, chatID int64, b bot.BotConnector) error {
	if data == "/start" && h.commandHandler != nil {
		if err := h.commandHandler.HandleCommandStart(ctx, chatID, h.bot); err != nil {
			return errors.Wrap(err, "failed to handle /start command")
		}
		return nil
	}
	if err := h.defaultHandler.HandleTextMessage(ctx, data, chatID, h.bot); err != nil {
		return errors.Wrap(err, "failed to handle text message in default handler")
	}
	return nil
}

func (h *BotxHandler) HandleCallbackData(ctx context.Context, data string, chatID int64, b bot.BotConnector) error {
	if strings.HasPrefix(data, "lang:") {
		if err := h.handleLanguage(ctx, chatID, data); err != nil {
			return errors.Wrap(err, "failed to handle language switch")
		}
		return nil
	}
	if strings.HasPrefix(data, fmt.Sprintf("%s:", bot.CallbackPrefixRoute)) {
		if err := h.handleRoute(ctx, chatID, data); err != nil {
			return errors.Wrap(err, "failed to handle route")
		}
		return nil
	}
	if strings.HasPrefix(data, fmt.Sprintf("%s:", bot.CallbackPrefixSubmit)) {
		if err := h.handleSubmit(ctx, chatID, data); err != nil {
			return errors.Wrap(err, "failed to handle submit")
		}
		return nil
	}
	if err := h.defaultHandler.HandleCallbackData(ctx, data, chatID, h.bot); err != nil {
		return errors.Wrap(err, "failed to handle callback data in default handler")
	}
	return nil
}

func (h *BotxHandler) HandleError(ctx context.Context, err error, chatID int64, b bot.BotConnector) error {
	if langCtx, langErr := h.withLanguage(ctx, chatID); langErr == nil {
		ctx = langCtx
	}
	if err := h.defaultHandler.HandleError(ctx, err, chatID, h.bot); err != nil {
		return errors.Wrap(err, "failed to handle error in default handler")
	}
	if err := h.renderer.pageError(ctx, chatID, err); err != nil {
		return errors.Wrap(err, "failed to render error page")
	}
	return nil
}

func (h *BotxHandler) Validate(ctx context.Context, chatID int64, url *url.URL, validator string, input string) (*bot.ValidateResult, error) {
	switch validator {
	default:
		return nil, errors.Wrapf(bot.ErrNotFound, "unknown validator: %s", validator)
	}
}

// code generated for pages

var (
	todoIDMatcher     = routepath.MustCompile("/todo/{ID}")
	todoDeleteMatcher = routepath.MustCompile("/todo/{ID}/delete")
	todoToggleMatcher = routepath.MustCompile("/todo/{ID}/toggle")
)

func (h *BotxHandler) onRoute(ctx context.Context, chatID int64, url *url.URL) error {
	paramsTodoID, okTodoID := todoIDMatcher.Match(url.Path)
	paramsTodoDelete, okTodoDelete := todoDeleteMatcher.Match(url.Path)
	paramsTodoToggle, okTodoToggle := todoToggleMatcher.Match(url.Path)

	switch {
	case url.Path == "/":
		params, err := ParseParametersPageRoot(url)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /")
		}
		state, err := h.sp.ProvideRootState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /")
		}
		if err := h.renderer.pageRoot(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /")
		}
	case url.Path == "/i18n":
		params, err := ParseParametersPageI18n(url)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /i18n")
		}
		state, err := h.sp.ProvideI18nState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /i18n")
		}
		if err := h.renderer.pageI18n(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /i18n")
		}
	case url.Path == "/todo/add":
		params, err := ParseParametersPageTodoAdd(url)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /todo/add")
		}
		if err := h.renderer.formTodoAdd(ctx, chatID, url, params); err != nil {
			return errors.Wrap(err, "failed to render form for page /todo/add")
		}
	case okTodoDelete:
		params, err := ParseParametersPageTodoDelete(paramsTodoDelete)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /todo/{ID}/delete")
		}
		state, err := h.sp.ProvideTodoDeleteState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /todo/{ID}/delete")
		}
		if err := h.renderer.pageTodoDelete(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /todo/{ID}/delete")
		}
	case okTodoToggle:
		params, err := ParseParametersPageTodoToggle(paramsTodoToggle)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /todo/{ID}/toggle")
		}
		state, err := h.sp.ProvideTodoToggleState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /todo/{ID}/toggle")
		}
		if err := h.renderer.pageTodoToggle(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /todo/{ID}/toggle")
		}
	case okTodoID:
		params, err := ParseParametersPageTodoID(paramsTodoID)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /todo/{ID}")
		}
		state, err := h.sp.ProvideTodoIDState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /todo/{ID}")
		}
		if err := h.renderer.pageTodoID(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /todo/{ID}")
		}
	default:
		return errors.Wrapf(bot.ErrNotFound, "unknown route: %s", url.String())
	}
	return nil
}

func (h *BotxHandler) onSubmit(ctx context.Context, chatID int64, url *url.URL) error {
	raw := url.Query().Get("values")
	if raw == "" {
		return errors.Wrap(bot.ErrBadRequest, "missing form values")
	}
	var values bot.FormValues
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return errors.Wrap(err, "failed to unmarshal form values")
	}

	switch {
	case url.Path == "/todo/add":
		params, err := ParseParametersPageTodoAdd(url)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for form /todo/add")
		}
		form, err := unmarshalFormTodoAdd(values)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal form for /todo/add")
		}
		state, err := h.sp.ProvideTodoAddState(ctx, chatID, form, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for form /todo/add")
		}
		if err := h.renderer.pageTodoAdd(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page for form /todo/add")
		}
	default:
		return errors.Wrapf(bot.ErrNotFound, "unknown form todo: %s", url.Path)
	}
	return nil
}

// url to params

func ParseParametersPageRoot(url *url.URL) (*ParametersPageRoot, error) {
	column, err := parseOptionalIntQuery(url, "column", defaultRootColumns)
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid column parameter: %s", err.Error())
	}
	page, err := parseOptionalIntQuery(url, "page", defaultRootPage)
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid page parameter: %s", err.Error())
	}
	row, err := parseOptionalIntQuery(url, "row", defaultRootRows)
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid row parameter: %s", err.Error())
	}
	return &ParametersPageRoot{
		column: column,
		page:   page,
		row:    row,
	}, nil
}

func ParseParametersPageI18n(url *url.URL) (*ParametersPageI18n, error) {
	return &ParametersPageI18n{}, nil
}

func ParseParametersPageTodoAdd(url *url.URL) (*ParametersPageTodoAdd, error) {
	return &ParametersPageTodoAdd{}, nil
}

func ParseParametersPageTodoID(params routepath.Params) (*ParametersPageTodoID, error) {
	id, err := parseParamInt64(params, "ID")
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid id parameter: %s", err.Error())
	}
	return &ParametersPageTodoID{
		ID: id,
	}, nil
}

func ParseParametersPageTodoDelete(params routepath.Params) (*ParametersPageTodoDelete, error) {
	id, err := parseParamInt64(params, "ID")
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid id parameter: %s", err.Error())
	}
	return &ParametersPageTodoDelete{
		ID: id,
	}, nil
}

func ParseParametersPageTodoToggle(params routepath.Params) (*ParametersPageTodoToggle, error) {
	id, err := parseParamInt64(params, "ID")
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid id parameter: %s", err.Error())
	}
	return &ParametersPageTodoToggle{
		ID: id,
	}, nil
}

// forms

type FormTodoAdd struct {
	title string
}

func (f *FormTodoAdd) GetTitle() string {
	return f.title
}

func unmarshalFormTodoAdd(values bot.FormValues) (*FormTodoAdd, error) {
	title, ok := values["title"]
	if !ok {
		return nil, errors.Wrap(bot.ErrBadRequest, "title is required")
	}
	return &FormTodoAdd{
		title: title,
	}, nil
}

// FormValidator

type FormValidator interface {
}

// StateProvider provides state views.
type StateProvider interface {
	ProvideRootState(ctx context.Context, chatID int64, parameters *ParametersPageRoot) (*StatePageRoot, error)
	ProvideI18nState(ctx context.Context, chatID int64, parameters *ParametersPageI18n) (*StatePageI18n, error)
	ProvideTodoAddState(ctx context.Context, chatID int64, form *FormTodoAdd, parameters *ParametersPageTodoAdd) (*StatePageTodoAdd, error)
	ProvideTodoIDState(ctx context.Context, chatID int64, parameters *ParametersPageTodoID) (*StatePageTodoID, error)
	ProvideTodoDeleteState(ctx context.Context, chatID int64, parameters *ParametersPageTodoDelete) (*StatePageTodoDelete, error)
	ProvideTodoToggleState(ctx context.Context, chatID int64, parameters *ParametersPageTodoToggle) (*StatePageTodoToggle, error)
}
type PageRenderer struct {
	b *bot.Bot
}

type ParametersPageRoot struct {
	column int
	page   int
	row    int
}

func (p *ParametersPageRoot) GetColumn() int {
	return p.column
}

func (p *ParametersPageRoot) GetPage() int {
	return p.page
}

func (p *ParametersPageRoot) GetRow() int {
	return p.row
}

type StatePageRoot struct {
	items []Todo
	total int
}

func NewStatePageRoot(items []Todo, total int) *StatePageRoot {
	return &StatePageRoot{
		items: items,
		total: total,
	}
}

func (s *StatePageRoot) GetItems() []Todo {
	return s.items
}

func (s *StatePageRoot) GetTotal() int {
	return s.total
}

func (p *PageRenderer) pageRoot(ctx context.Context, chatID int64, state *StatePageRoot, parameters *ParametersPageRoot) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n%v\n%v\n", cond(len(state.GetItems()) == 0, i18n(ctx, chatID, "content.todo.empty"), ""), forEach(
			state.GetItems(),
			func(index int, item Todo) string {
				return fmt.Sprintf(
					"%d. %s<code>%s</code>\n",
					index+1,
					cond(item.GetDone(), "[x] ", "[ ] "),
					item.GetTitle(),
				)
			}), cond(len(state.GetItems()) == 0, "", i18n(ctx, chatID, "content.todo.select"))),
		ParseMode: "HTML",
		ButtonGrid: appendButtonGrids(
			pagination(
				parameters.GetColumn(),
				parameters.GetRow(),
				state.GetTotal(),
				parameters.GetPage(),
				state.GetItems(),
				func(item Todo) bot.Button {
					return bot.Button{
						Label:        fmt.Sprintf("%v", cond(item.GetDone(), "[x] "+item.GetTitle(), "[ ] "+item.GetTitle())),
						CallbackData: bot.CallbackData(fmt.Sprintf("route:/todo/%v", item.ID)),
					}
				},
				fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.prev")),
				fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.next")),
			),
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.add_button")), CallbackData: bot.CallbackData("route:/todo/add")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.i18n")), CallbackData: bot.CallbackData("route:/i18n")},
				},
			},
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.back")), CallbackData: bot.CallbackData("route:back")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.home")), CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageRoot")
	}
	return nil
}

type ParametersPageI18n struct {
}

type StatePageI18n struct {
}

func (p *PageRenderer) pageI18n(ctx context.Context, chatID int64, state *StatePageI18n, parameters *ParametersPageI18n) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v", i18n(ctx, chatID, "content.i18n.title")),
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.i18n.zh")), CallbackData: bot.CallbackData("lang:zh-hans")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.i18n.en")), CallbackData: bot.CallbackData("lang:en")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.i18n.es")), CallbackData: bot.CallbackData("lang:es")},
				},
			},
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.back")), CallbackData: bot.CallbackData("route:back")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.home")), CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageI18n")
	}
	return nil
}

type ParametersPageTodoAdd struct {
}

type StatePageTodoAdd struct {
	success bool
	errMsg  string
}

func NewStatePageTodoAdd(success bool, errMsg string) *StatePageTodoAdd {
	return &StatePageTodoAdd{
		success: success,
		errMsg:  errMsg,
	}
}

func (s *StatePageTodoAdd) GetSuccess() bool {
	return s.success
}

func (s *StatePageTodoAdd) GetError() string {
	return s.errMsg
}

func (p *PageRenderer) pageTodoAdd(ctx context.Context, chatID int64, state *StatePageTodoAdd, parameters *ParametersPageTodoAdd) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n", cond(
			state.GetSuccess(),
			i18n(ctx, chatID, "content.todo.add.success"),
			fmt.Sprintf(i18n(ctx, chatID, "content.todo.add.fail"), state.GetError()),
		)),
		ButtonGrid: [][]bot.Button{
			{
				{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.back")), CallbackData: bot.CallbackData("route:back")},
				{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.home")), CallbackData: bot.CallbackData("route:/")},
			},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageTodoAdd")
	}
	return nil
}

func (p *PageRenderer) formTodoAdd(ctx context.Context, chatID int64, url *url.URL, parameters *ParametersPageTodoAdd) error {
	form := &bot.Form{
		URL: url,
		Idx: 0,
		Fields: []bot.FormField{
			{
				ID:    "title",
				Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.add.title_label")),
				Input: &bot.FormFieldInput{
					Schema: &bot.FormSchema{
						Type:   "string",
						Format: "",
					},
					Tip: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.add.title_tip")),
				},
			},
		},
	}
	if err := p.b.SendForm(ctx, chatID, form); err != nil {
		return errors.Wrap(err, "failed to send formTodoAdd")
	}
	return nil
}

type ParametersPageTodoID struct {
	ID int64
}

func (p *ParametersPageTodoID) GetID() int64 {
	return p.ID
}

type StatePageTodoID struct {
	todo Todo
}

func NewStatePageTodoID(todo Todo) *StatePageTodoID {
	return &StatePageTodoID{
		todo: todo,
	}
}

func (s *StatePageTodoID) GetID() int64 {
	return s.todo.ID
}

func (s *StatePageTodoID) GetTitle() string {
	return s.todo.GetTitle()
}

func (s *StatePageTodoID) GetDone() bool {
	return s.todo.GetDone()
}

func (p *PageRenderer) pageTodoID(ctx context.Context, chatID int64, state *StatePageTodoID, parameters *ParametersPageTodoID) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text:      fmt.Sprintf("%v<code>%v</code>\n%v%v\n", i18n(ctx, chatID, "content.todo.detail.title_prefix"), state.GetTitle(), i18n(ctx, chatID, "content.todo.detail.status_prefix"), cond(state.GetDone(), i18n(ctx, chatID, "content.todo.detail.status_done"), i18n(ctx, chatID, "content.todo.detail.status_open"))),
		ParseMode: "HTML",
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.detail.toggle_button")), CallbackData: bot.CallbackData(fmt.Sprintf("route:/todo/%v/toggle", parameters.GetID()))},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.detail.delete_button")), CallbackData: bot.CallbackData(fmt.Sprintf("route:/todo/%v/delete", parameters.GetID()))},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.detail.back_list")), CallbackData: bot.CallbackData("route:/")},
				},
			},
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.back")), CallbackData: bot.CallbackData("route:back")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.home")), CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageTodoID")
	}
	return nil
}

type ParametersPageTodoDelete struct {
	ID int64
}

func (p *ParametersPageTodoDelete) GetID() int64 {
	return p.ID
}

type StatePageTodoDelete struct {
	success bool
	errMsg  string
}

func NewStatePageTodoDelete(success bool, errMsg string) *StatePageTodoDelete {
	return &StatePageTodoDelete{
		success: success,
		errMsg:  errMsg,
	}
}

func (s *StatePageTodoDelete) GetSuccess() bool {
	return s.success
}

func (s *StatePageTodoDelete) GetError() string {
	return s.errMsg
}

func (p *PageRenderer) pageTodoDelete(ctx context.Context, chatID int64, state *StatePageTodoDelete, parameters *ParametersPageTodoDelete) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n", cond(
			state.GetSuccess(),
			i18n(ctx, chatID, "content.todo.delete.success"),
			fmt.Sprintf(i18n(ctx, chatID, "content.todo.delete.fail"), state.GetError()),
		)),
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.detail.back_list")), CallbackData: bot.CallbackData("route:/")},
				},
			},
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.back")), CallbackData: bot.CallbackData("route:back")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.home")), CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageTodoDelete")
	}
	return nil
}

type ParametersPageTodoToggle struct {
	ID int64
}

func (p *ParametersPageTodoToggle) GetID() int64 {
	return p.ID
}

type StatePageTodoToggle struct {
	success bool
	errMsg  string
	done    bool
}

func NewStatePageTodoToggle(success bool, errMsg string, done bool) *StatePageTodoToggle {
	return &StatePageTodoToggle{
		success: success,
		errMsg:  errMsg,
		done:    done,
	}
}

func (s *StatePageTodoToggle) GetSuccess() bool {
	return s.success
}

func (s *StatePageTodoToggle) GetError() string {
	return s.errMsg
}

func (s *StatePageTodoToggle) GetDone() bool {
	return s.done
}

func (p *PageRenderer) pageTodoToggle(ctx context.Context, chatID int64, state *StatePageTodoToggle, parameters *ParametersPageTodoToggle) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n", cond(
			state.GetSuccess(),
			cond(state.GetDone(), i18n(ctx, chatID, "content.todo.toggle.done"), i18n(ctx, chatID, "content.todo.toggle.open")),
			fmt.Sprintf(i18n(ctx, chatID, "content.todo.toggle.fail"), state.GetError()),
		)),
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.toggle.back_todo")), CallbackData: bot.CallbackData(fmt.Sprintf("route:/todo/%v", parameters.GetID()))},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.todo.detail.back_list")), CallbackData: bot.CallbackData("route:/")},
				},
			},
			[][]bot.Button{
				{
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.back")), CallbackData: bot.CallbackData("route:back")},
					{Label: fmt.Sprintf("%v", i18n(ctx, chatID, "content.nav.home")), CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageTodoToggle")
	}
	return nil
}

func (p *PageRenderer) pageError(ctx context.Context, chatID int64, err error) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%s", err.Error()),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageError")
	}
	return nil
}

func cond[T any](condition bool, a, b T) T {
	if condition {
		return a
	}
	return b
}

func appendButtonGrids(grids ...[][]bot.Button) [][]bot.Button {
	var result [][]bot.Button
	for _, grid := range grids {
		result = append(result, grid...)
	}
	return result
}

func forEach[T any](slice []T, fn func(index int, item T) string) string {
	var sb strings.Builder
	for i, v := range slice {
		sb.WriteString(fn(i, v))
	}
	return sb.String()
}

const i18nDefault = "en"

var i18nEntries = map[string]map[string]string{
	"content.i18n.en": {
		"en":      "English",
		"es":      "English",
		"zh-hans": "English",
	},
	"content.i18n.es": {
		"en":      "espanol",
		"es":      "espanol",
		"zh-hans": "espanol",
	},
	"content.i18n.title": {
		"en":      "Choose a language",
		"es":      "Elige un idioma",
		"zh-hans": "请选择语言",
	},
	"content.i18n.zh": {
		"en":      "中文",
		"es":      "中文",
		"zh-hans": "中文",
	},
	"content.nav.back": {
		"en":      "⬅️ Back",
		"es":      "⬅️ Atras",
		"zh-hans": "⬅️ 返回",
	},
	"content.nav.home": {
		"en":      "🏠 Home",
		"es":      "🏠 Inicio",
		"zh-hans": "🏠 主页",
	},
	"content.nav.i18n": {
		"en":      "🌐 Language",
		"es":      "🌐 Idioma",
		"zh-hans": "🌐 语言",
	},
	"content.todo.add.fail": {
		"en":      "Failed to add todo: %s ❌",
		"es":      "No se pudo agregar la tarea: %s ❌",
		"zh-hans": "添加待办失败: %s ❌",
	},
	"content.todo.add.success": {
		"en":      "Todo added. Use the buttons to go back. ✅",
		"es":      "Tarea agregada. Usa los botones para volver. ✅",
		"zh-hans": "待办已添加。使用按钮返回。✅",
	},
	"content.todo.add.title_label": {
		"en":      "Title",
		"es":      "Titulo",
		"zh-hans": "标题",
	},
	"content.todo.add.title_tip": {
		"en":      "Enter a short todo title.",
		"es":      "Ingresa un titulo corto.",
		"zh-hans": "请输入简短的待办标题。",
	},
	"content.todo.add_button": {
		"en":      "➕ Add Todo",
		"es":      "➕ Agregar tarea",
		"zh-hans": "➕ 添加待办",
	},
	"content.todo.delete.fail": {
		"en":      "Failed to delete todo: %s ❌",
		"es":      "No se pudo eliminar la tarea: %s ❌",
		"zh-hans": "删除待办失败: %s ❌",
	},
	"content.todo.delete.success": {
		"en":      "Todo deleted. 🧹",
		"es":      "Tarea eliminada. 🧹",
		"zh-hans": "待办已删除。🧹",
	},
	"content.todo.detail.back_list": {
		"en":      "📋 Back to List",
		"es":      "📋 Volver a la lista",
		"zh-hans": "📋 返回列表",
	},
	"content.todo.detail.delete_button": {
		"en":      "🗑️ Delete",
		"es":      "🗑️ Eliminar",
		"zh-hans": "🗑️ 删除",
	},
	"content.todo.detail.status_done": {
		"en":      "done ✅",
		"es":      "completada ✅",
		"zh-hans": "已完成 ✅",
	},
	"content.todo.detail.status_open": {
		"en":      "open ⏳",
		"es":      "pendiente ⏳",
		"zh-hans": "未完成 ⏳",
	},
	"content.todo.detail.status_prefix": {
		"en":      "Status: ",
		"es":      "Estado: ",
		"zh-hans": "状态: ",
	},
	"content.todo.detail.title_prefix": {
		"en":      "Title: ",
		"es":      "Titulo: ",
		"zh-hans": "标题: ",
	},
	"content.todo.detail.toggle_button": {
		"en":      "✅ Toggle Done",
		"es":      "✅ Alternar estado",
		"zh-hans": "✅ 切换完成状态",
	},
	"content.todo.empty": {
		"en":      "No todos yet. Add one below. ✨",
		"es":      "No hay tareas aun. Agrega una abajo. ✨",
		"zh-hans": "暂无待办事项，添加一个吧。✨",
	},
	"content.todo.next": {
		"en":      "Next ➡️",
		"es":      "Siguiente ➡️",
		"zh-hans": "下一页 ➡️",
	},
	"content.todo.prev": {
		"en":      "⬅️ Prev",
		"es":      "⬅️ Anterior",
		"zh-hans": "⬅️ 上一页",
	},
	"content.todo.select": {
		"en":      "Select a todo to view details. 👇",
		"es":      "Selecciona una tarea para ver detalles. 👇",
		"zh-hans": "选择一个待办查看详情。👇",
	},
	"content.todo.toggle.back_todo": {
		"en":      "📝 Back to Todo",
		"es":      "📝 Volver a la tarea",
		"zh-hans": "📝 返回待办",
	},
	"content.todo.toggle.done": {
		"en":      "Todo marked done. ✅",
		"es":      "Tarea marcada como completada. ✅",
		"zh-hans": "待办标记为已完成。✅",
	},
	"content.todo.toggle.fail": {
		"en":      "Failed to update todo: %s ❌",
		"es":      "No se pudo actualizar la tarea: %s ❌",
		"zh-hans": "更新待办失败: %s ❌",
	},
	"content.todo.toggle.open": {
		"en":      "Todo marked open. ⏳",
		"es":      "Tarea marcada como pendiente. ⏳",
		"zh-hans": "待办标记为未完成。⏳",
	},
}

func i18n(ctx context.Context, _ int64, key string) string {
	lang := strings.ToLower(bot.LanguageFromContext(ctx))
	if lang == "" {
		lang = i18nDefault
	}
	if value := i18nLookup(key, lang); value != "" {
		return value
	}
	if lang != i18nDefault {
		if value := i18nLookup(key, i18nDefault); value != "" {
			return value
		}
	}
	return i18nFallback(key)
}

func i18nStatic(key string) string {
	if value := i18nLookup(key, i18nDefault); value != "" {
		return value
	}
	return i18nFallback(key)
}

func i18nLookup(key string, lang string) string {
	entry, ok := i18nEntries[key]
	if !ok {
		return ""
	}
	return entry[lang]
}

func i18nFallback(key string) string {
	entry, ok := i18nEntries[key]
	if !ok {
		return key
	}
	for _, value := range entry {
		if value != "" {
			return value
		}
	}
	return key
}

var navbar = []bot.Button{{Label: fmt.Sprintf("%v", i18nStatic("content.nav.back")), CallbackData: bot.CallbackData("route:back")}, {Label: fmt.Sprintf("%v", i18nStatic("content.nav.home")), CallbackData: bot.CallbackData("route:/")}}

func pagination[T any](columns int, rows int, total int, page int, items []T, castFunc func(item T) bot.Button, prevLabel string, nextLabel string) [][]bot.Button {
	var grid [][]bot.Button
	if rows <= 0 || columns <= 0 {
		return grid
	}

	pageSize := rows * columns
	isLastPage := (page+1)*pageSize >= total

	for r := 0; r < rows; r++ {
		var row []bot.Button
		for c := 0; c < columns; c++ {
			index := r*columns + c
			if index >= len(items) {
				break
			}
			row = append(row, castFunc(items[index]))
		}
		if len(row) == 0 {
			break
		}
		grid = append(grid, row)
	}

	ctrlRow := []bot.Button{}

	if page != 0 {
		ctrlRow = append(ctrlRow, bot.Button{
			Label:        prevLabel,
			CallbackData: bot.CallbackData(fmt.Sprintf("/?row=%d&column=%d&page=%d", rows, columns, page-1)),
		})
	}

	if !isLastPage {
		ctrlRow = append(ctrlRow, bot.Button{
			Label:        nextLabel,
			CallbackData: bot.CallbackData(fmt.Sprintf("/?row=%d&column=%d&page=%d", rows, columns, page+1)),
		})
	}

	if len(ctrlRow) != 0 {
		grid = append(grid, ctrlRow)
	}

	return grid
}

const (
	defaultRootColumns = 2
	defaultRootPage    = 0
	defaultRootRows    = 5
)

func parseOptionalIntQuery(url *url.URL, key string, defaultValue int) (int, error) {
	value := url.Query().Get(key)
	if value == "" {
		return defaultValue, nil
	}
	return ToInt(value)
}

func parseOptionalInt64Query(url *url.URL, key string, defaultValue int64) (int64, error) {
	value := url.Query().Get(key)
	if value == "" {
		return defaultValue, nil
	}
	return ToInt64(value)
}

func parseOptionalStringQuery(url *url.URL, key string, defaultValue string) string {
	value := url.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func parseParamInt64(params routepath.Params, key string) (int64, error) {
	value, ok := params.Get(key)
	if !ok || value == "" {
		return 0, errors.Wrapf(bot.ErrBadRequest, "missing %s parameter", key)
	}
	return ToInt64(value)
}

func ToInt(s string) (int, error) {
	var i int
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func ToInt32(s string) (int32, error) {
	var i int32
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func ToInt64(s string) (int64, error) {
	var i int64
	_, err := fmt.Sscanf(s, "%d", &i)
	if err != nil {
		return 0, err
	}
	return i, nil
}

func ptr[T any](v T) *T {
	return &v
}
