package common

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

type Address struct {
	ID      int64
	address string
	name    string
}

func NewAddress(ID int64, address string, name string) *Address {
	return &Address{
		ID:      ID,
		address: address,
		name:    name,
	}
}

func (v Address) GetAddress() string {
	return v.address
}

func (v Address) GetName() string {
	return v.name
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
	case "validateAddressOrName":
		return h.formValidator.ValidateFormAddressAdd(ctx, chatID, url, input)
	default:
		return nil, errors.Wrapf(bot.ErrNotFound, "unknown validator: %s", validator)
	}
}

// code generated for pages

var (
	addressIDMatcher     = routepath.MustCompile("/address/{ID}")
	addressDeleteMatcher = routepath.MustCompile("/address/{ID}/delete")
	addressEditMatcher   = routepath.MustCompile("/address/{ID}/edit")
)

func (h *BotxHandler) onRoute(ctx context.Context, chatID int64, url *url.URL) error {
	paramsAddressID, okAddressID := addressIDMatcher.Match(url.Path)
	paramsAddressDelete, okAddressDelete := addressDeleteMatcher.Match(url.Path)
	paramsAddressEdit, okAddressEdit := addressEditMatcher.Match(url.Path)

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
	case url.Path == "/address":
		params, err := ParseParametersPageAddress(url)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /address")
		}
		state, err := h.sp.ProvideAddressState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /address")
		}
		if err := h.renderer.pageAddress(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /address")
		}
	case url.Path == "/address/add":
		params, err := ParseParametersPageAddressAdd(url)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /address/add")
		}
		if err := h.renderer.formAddressAdd(ctx, chatID, url, params); err != nil {
			return errors.Wrap(err, "failed to render form for page /address/add")
		}
	case okAddressDelete:
		params, err := ParseParametersPageAddressDelete(paramsAddressDelete)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /address/{ID}/delete")
		}
		state, err := h.sp.ProvideAddressDeleteState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /address/{ID}/delete")
		}
		if err := h.renderer.pageAddressDelete(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /address/{ID}/delete")
		}
	case okAddressEdit:
		params, err := ParseParametersPageAddressEdit(url, paramsAddressEdit)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /address/{ID}/edit")
		}
		if err := h.renderer.formAddressEdit(ctx, chatID, url, params); err != nil {
			return errors.Wrap(err, "failed to render form for page /address/{ID}/edit")
		}
	case okAddressID:
		params, err := ParseParametersPageAddressID(paramsAddressID)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for page /address/{ID}")
		}
		state, err := h.sp.ProvideAddressIDState(ctx, chatID, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for page /address/{ID}")
		}
		if err := h.renderer.pageAddressID(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page /address/{ID}")
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

	paramsAddressEdit, okAddressEdit := addressEditMatcher.Match(url.Path)

	switch {
	case url.Path == "/address/add":
		params, err := ParseParametersPageAddressAdd(url)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for form /address/add")
		}
		form, err := unmarshalFormAddressAdd(values)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal form for /address/add")
		}
		state, err := h.sp.ProvideAddressAddState(ctx, chatID, form, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for form /address/add")
		}
		if err := h.renderer.pageAddressAdd(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page for form /address/add")
		}
	case okAddressEdit:
		params, err := ParseParametersPageAddressEdit(url, paramsAddressEdit)
		if err != nil {
			return errors.Wrap(err, "invalid parameters for form /address/{ID}/edit")
		}
		form, err := unmarshalFormAddressEdit(values)
		if err != nil {
			return errors.Wrap(err, "failed to unmarshal form for /address/{ID}/edit")
		}
		state, err := h.sp.ProvideAddressEditState(ctx, chatID, form, params)
		if err != nil {
			return errors.Wrap(err, "failed to provide state for form /address/{ID}/edit")
		}
		if err := h.renderer.pageAddressEdit(ctx, chatID, state, params); err != nil {
			return errors.Wrap(err, "failed to render page for form /address/{ID}/edit")
		}
	default:
		return errors.Wrapf(bot.ErrNotFound, "unknown form address: %s", url.Path)
	}
	return nil
}

// url to params

func ParseParametersPageRoot(url *url.URL) (*ParametersPageRoot, error) {
	return &ParametersPageRoot{}, nil
}

func ParseParametersPageAddress(url *url.URL) (*ParametersPageAddress, error) {
	column, err := parseOptionalIntQuery(url, "column", defaultAddressColumns)
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid column parameter: %s", err.Error())
	}
	page, err := parseOptionalIntQuery(url, "page", defaultAddressPage)
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid page parameter: %s", err.Error())
	}
	row, err := parseOptionalIntQuery(url, "row", defaultAddressRows)
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid row parameter: %s", err.Error())
	}
	return &ParametersPageAddress{
		column: column,
		page:   page,
		row:    row,
	}, nil
}

func ParseParametersPageAddressAdd(url *url.URL) (*ParametersPageAddressAdd, error) {
	return &ParametersPageAddressAdd{}, nil
}

func ParseParametersPageAddressID(params routepath.Params) (*ParametersPageAddressID, error) {
	id, err := parseParamInt64(params, "ID")
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid id parameter: %s", err.Error())
	}
	return &ParametersPageAddressID{
		ID: id,
	}, nil
}

func ParseParametersPageAddressDelete(params routepath.Params) (*ParametersPageAddressDelete, error) {
	id, err := parseParamInt64(params, "ID")
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid id parameter: %s", err.Error())
	}
	return &ParametersPageAddressDelete{
		ID: id,
	}, nil
}

func ParseParametersPageAddressEdit(url *url.URL, params routepath.Params) (*ParametersPageAddressEdit, error) {
	id, err := parseParamInt64(params, "ID")
	if err != nil {
		return nil, errors.Wrapf(bot.ErrBadRequest, "invalid id parameter: %s", err.Error())
	}
	field := url.Query().Get("field")
	if field == "" {
		return nil, errors.Wrap(bot.ErrBadRequest, "missing field parameter")
	}
	return &ParametersPageAddressEdit{
		ID:    id,
		field: field,
	}, nil
}

// forms

type FormAddressAdd struct {
	address string
}

func (f *FormAddressAdd) GetAddress() string {
	return f.address
}

func unmarshalFormAddressAdd(values bot.FormValues) (*FormAddressAdd, error) {
	address, ok := values["address"]
	if !ok {
		return nil, errors.Wrap(bot.ErrBadRequest, "address is required")
	}
	return &FormAddressAdd{
		address: address,
	}, nil
}

type FormAddressEdit struct {
	value string
}

func (f *FormAddressEdit) GetValue() string {
	return f.value
}

func unmarshalFormAddressEdit(values bot.FormValues) (*FormAddressEdit, error) {
	value, ok := values["value"]
	if !ok {
		return nil, errors.Wrap(bot.ErrBadRequest, "value is required")
	}
	return &FormAddressEdit{
		value: value,
	}, nil
}

// FormValidator

type FormValidator interface {
	ValidateFormAddressAdd(ctx context.Context, chatID int64, url *url.URL, input string) (*bot.ValidateResult, error)
}

// StateProvider provides state views.
type StateProvider interface {
	ProvideRootState(ctx context.Context, chatID int64, parameters *ParametersPageRoot) (*StatePageRoot, error)
	ProvideAddressState(ctx context.Context, chatID int64, parameters *ParametersPageAddress) (*StatePageAddress, error)
	ProvideAddressAddState(ctx context.Context, chatID int64, form *FormAddressAdd, parameters *ParametersPageAddressAdd) (*StatePageAddressAdd, error)
	ProvideAddressIDState(ctx context.Context, chatID int64, parameters *ParametersPageAddressID) (*StatePageAddressID, error)
	ProvideAddressDeleteState(ctx context.Context, chatID int64, parameters *ParametersPageAddressDelete) (*StatePageAddressDelete, error)
	ProvideAddressEditState(ctx context.Context, chatID int64, form *FormAddressEdit, parameters *ParametersPageAddressEdit) (*StatePageAddressEdit, error)
}
type PageRenderer struct {
	b *bot.Bot
}

type ParametersPageRoot struct {
}

type StatePageRoot struct {
}

func (p *PageRenderer) pageRoot(ctx context.Context, chatID int64, state *StatePageRoot, parameters *ParametersPageRoot) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: "欢迎使用地址管理机器人！请选择下方按钮进行操作。",
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: "管理地址", CallbackData: bot.CallbackData("route:/address")},
				},
			},
			[][]bot.Button{
				{
					{Label: "返回", CallbackData: bot.CallbackData("route:back")},
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageRoot")
	}
	return nil
}

type ParametersPageAddress struct {
	column int
	page   int
	row    int
}

func (p *ParametersPageAddress) GetColumn() int {
	return p.column
}

func (p *ParametersPageAddress) GetPage() int {
	return p.page
}

func (p *ParametersPageAddress) GetRow() int {
	return p.row
}

type StatePageAddress struct {
	items []Address
	total int
}

func NewStatePageAddress(items []Address, total int) *StatePageAddress {
	return &StatePageAddress{
		items: items,
		total: total,
	}
}

func (s *StatePageAddress) GetItems() []Address {
	return s.items
}

func (s *StatePageAddress) GetTotal() int {
	return s.total
}

func (p *PageRenderer) pageAddress(ctx context.Context, chatID int64, state *StatePageAddress, parameters *ParametersPageAddress) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n%v\n%v\n", cond(len(state.GetItems()) == 0, "暂无地址，请添加地址", ""), forEach(
			state.GetItems(),
			func(index int, item Address) string {
				return fmt.Sprintf(
					"%d. %s<code>%s</code>\n",
					index+1,
					cond(item.GetName() == "", "", item.GetName()+" "),
					item.GetAddress(),
				)
			}), cond(len(state.GetItems()) == 0, "", "请点击序号对应的按钮对地址进行操作")),
		ParseMode: "HTML",
		ButtonGrid: appendButtonGrids(
			pagination(
				parameters.GetColumn(),
				parameters.GetRow(),
				state.GetTotal(),
				parameters.GetPage(),
				state.GetItems(),
				func(item Address) bot.Button {
					return bot.Button{
						Label:        fmt.Sprintf("%v", cond(item.GetName() == "", item.GetAddress(), item.GetName())),
						CallbackData: bot.CallbackData(fmt.Sprintf("route:/address/%v", item.ID)),
					}
				},
				"上一页",
				"下一页",
			),
			[][]bot.Button{
				{
					{Label: "添加新地址", CallbackData: bot.CallbackData("route:/address/add")},
				},
			},
			[][]bot.Button{
				{
					{Label: "返回", CallbackData: bot.CallbackData("route:back")},
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageAddress")
	}
	return nil
}

type ParametersPageAddressAdd struct {
}

type StatePageAddressAdd struct {
	success bool
	errMsg  string
}

func NewStatePageAddressAdd(success bool, errMsg string) *StatePageAddressAdd {
	return &StatePageAddressAdd{
		success: success,
		errMsg:  errMsg,
	}
}

func (s *StatePageAddressAdd) GetSuccess() bool {
	return s.success
}

func (s *StatePageAddressAdd) GetError() string {
	return s.errMsg
}

func (p *PageRenderer) pageAddressAdd(ctx context.Context, chatID int64, state *StatePageAddressAdd, parameters *ParametersPageAddressAdd) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n", cond(
			state.GetSuccess(),
			"地址添加成功！点击下方按钮返回地址列表",
			fmt.Sprintf("地址添加失败: %s", state.GetError()),
		)),
		ButtonGrid: [][]bot.Button{
			{
				{Label: "返回", CallbackData: bot.CallbackData("route:back")},
				{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
			},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageAddressAdd")
	}
	return nil
}

func (p *PageRenderer) formAddressAdd(ctx context.Context, chatID int64, url *url.URL, parameters *ParametersPageAddressAdd) error {
	form := &bot.Form{
		URL: url,
		Idx: 0,
		Fields: []bot.FormField{
			{
				ID:    "address",
				Label: "地址",
				Input: &bot.FormFieldInput{
					Schema: &bot.FormSchema{
						Type:   "string",
						Format: "",
					},
					Tip: "请输入地址，如果需要备注请用空格分隔。例如:\nTxxxxxxxx\nTxxxxxxxx 备注\n",
				},
				Validator: ptr("validateAddressOrName"),
			},
		},
	}
	if err := p.b.SendForm(ctx, chatID, form); err != nil {
		return errors.Wrap(err, "failed to send formAddressAdd")
	}
	return nil
}

type ParametersPageAddressID struct {
	ID int64
}

func (p *ParametersPageAddressID) GetID() int64 {
	return p.ID
}

type StatePageAddressID struct {
	address Address
}

func NewStatePageAddressID(address Address) *StatePageAddressID {
	return &StatePageAddressID{
		address: address,
	}
}

func (s *StatePageAddressID) GetID() int64 {
	return s.address.ID
}

func (s *StatePageAddressID) GetAddress() string {
	return s.address.GetAddress()
}

func (s *StatePageAddressID) GetName() string {
	return s.address.GetName()
}

func (p *PageRenderer) pageAddressID(ctx context.Context, chatID int64, state *StatePageAddressID, parameters *ParametersPageAddressID) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text:      fmt.Sprintf("地址: %v\n备注: %v\n\n请点击下列按钮进行操作\n", state.GetAddress(), state.GetName()),
		ParseMode: "HTML",
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
					{Label: "删除地址", CallbackData: bot.CallbackData(fmt.Sprintf("route:/address/%v/delete", parameters.GetID()))},
					{Label: "编辑备注", CallbackData: bot.CallbackData(fmt.Sprintf("route:/address/%v/edit?field=name", parameters.GetID()))},
					{Label: "编辑地址", CallbackData: bot.CallbackData(fmt.Sprintf("route:/address/%v/edit?field=address", parameters.GetID()))},
				},
			},
			[][]bot.Button{
				{
					{Label: "返回", CallbackData: bot.CallbackData("route:back")},
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageAddressID")
	}
	return nil
}

type ParametersPageAddressDelete struct {
	ID int64
}

func (p *ParametersPageAddressDelete) GetID() int64 {
	return p.ID
}

type StatePageAddressDelete struct {
	success bool
	errMsg  string
}

func NewStatePageAddressDelete(success bool, errMsg string) *StatePageAddressDelete {
	return &StatePageAddressDelete{
		success: success,
		errMsg:  errMsg,
	}
}

func (s *StatePageAddressDelete) GetSuccess() bool {
	return s.success
}

func (s *StatePageAddressDelete) GetError() string {
	return s.errMsg
}

func (p *PageRenderer) pageAddressDelete(ctx context.Context, chatID int64, state *StatePageAddressDelete, parameters *ParametersPageAddressDelete) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n", cond(
			state.GetSuccess(),
			"地址删除成功！点击下方按钮返回地址列表",
			fmt.Sprintf("地址删除失败: %s", state.GetError()),
		)),
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: "返回地址列表", CallbackData: bot.CallbackData("route:/address")},
					{Label: "返回地址详情", CallbackData: bot.CallbackData(fmt.Sprintf("route:/address/%v", parameters.GetID()))},
				},
			},
			[][]bot.Button{
				{
					{Label: "返回", CallbackData: bot.CallbackData("route:back")},
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageAddressDelete")
	}
	return nil
}

type ParametersPageAddressEdit struct {
	ID    int64
	field string
}

func (p *ParametersPageAddressEdit) GetID() int64 {
	return p.ID
}

func (p *ParametersPageAddressEdit) GetField() string {
	return p.field
}

type StatePageAddressEdit struct {
	success bool
	errMsg  string
}

func NewStatePageAddressEdit(success bool, errMsg string) *StatePageAddressEdit {
	return &StatePageAddressEdit{
		success: success,
		errMsg:  errMsg,
	}
}

func (s *StatePageAddressEdit) GetSuccess() bool {
	return s.success
}

func (s *StatePageAddressEdit) GetError() string {
	return s.errMsg
}

func (p *PageRenderer) pageAddressEdit(ctx context.Context, chatID int64, state *StatePageAddressEdit, parameters *ParametersPageAddressEdit) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("%v\n", cond(
			state.GetSuccess(),
			"地址修改成功！点击下方按钮返回地址详情",
			fmt.Sprintf("地址修改失败: %s", state.GetError()),
		)),
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: "返回地址详情", CallbackData: bot.CallbackData(fmt.Sprintf("route:/address/%v", parameters.GetID()))},
					{Label: "返回地址列表", CallbackData: bot.CallbackData("route:/address")},
					{Label: "重新编辑", CallbackData: bot.CallbackData(fmt.Sprintf("route:/address/%v/edit?field=%v", parameters.GetID(), parameters.GetField()))},
				},
			},
			[][]bot.Button{
				{
					{Label: "返回", CallbackData: bot.CallbackData("route:back")},
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
	}); err != nil {
		return errors.Wrap(err, "failed to send page view message pageAddressEdit")
	}
	return nil
}

func (p *PageRenderer) formAddressEdit(ctx context.Context, chatID int64, url *url.URL, parameters *ParametersPageAddressEdit) error {
	form := &bot.Form{
		URL: url,
		Idx: 0,
		Fields: []bot.FormField{
			{
				ID:    "value",
				Label: "内容",
				Input: &bot.FormFieldInput{
					Schema: &bot.FormSchema{
						Type:   "string",
						Format: "",
					},
					Tip: fmt.Sprintf("%v", cond(parameters.GetField() == "name", "请输入新的备注名称", "请输入新的地址")),
				},
			},
		},
	}
	if err := p.b.SendForm(ctx, chatID, form); err != nil {
		return errors.Wrap(err, "failed to send formAddressEdit")
	}
	return nil
}

func (p *PageRenderer) pageError(ctx context.Context, chatID int64, err error) error {
	if err := p.b.SendMessage(ctx, chatID, &bot.Message{
		Text: fmt.Sprintf("错误信息: %v", err.Error()),
		ButtonGrid: appendButtonGrids(
			[][]bot.Button{
				{
					{Label: "返回上一页", CallbackData: bot.CallbackData("route:back")},
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
				},
			},
			[][]bot.Button{
				{
					{Label: "返回", CallbackData: bot.CallbackData("route:back")},
					{Label: "返回主页", CallbackData: bot.CallbackData("route:/")},
				},
			},
		),
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

var navbar = []bot.Button{{Label: "返回", CallbackData: bot.CallbackData("route:back")}, {Label: "返回主页", CallbackData: bot.CallbackData("route:/")}}

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
			CallbackData: bot.CallbackData(fmt.Sprintf("/address?row=%d&column=%d&page=%d", rows, columns, page-1)),
		})
	}

	if !isLastPage {
		ctrlRow = append(ctrlRow, bot.Button{
			Label:        nextLabel,
			CallbackData: bot.CallbackData(fmt.Sprintf("/address?row=%d&column=%d&page=%d", rows, columns, page+1)),
		})
	}

	if len(ctrlRow) != 0 {
		grid = append(grid, ctrlRow)
	}

	return grid
}

const (
	defaultAddressColumns = 2
	defaultAddressPage    = 0
	defaultAddressRows    = 5
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
