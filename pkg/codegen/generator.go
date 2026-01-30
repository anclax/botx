package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/getkin/kin-openapi/openapi3"
)

type Generator struct {
	doc *Doc
}

func NewGenerator(doc *Doc) *Generator {
	return &Generator{doc: doc}
}

func Generate(doc *Doc) ([]byte, error) {
	return NewGenerator(doc).Generate()
}

func (g *Generator) Generate() ([]byte, error) {
	gen := newGeneratorContext(g.doc)
	if err := gen.prepare(); err != nil {
		return nil, err
	}
	content, err := gen.render()
	if err != nil {
		return nil, err
	}
	return format.Source(content)
}

func (g *Generator) GenerateRaw() ([]byte, error) {
	gen := newGeneratorContext(g.doc)
	if err := gen.prepare(); err != nil {
		return nil, err
	}
	return gen.render()
}

func ResolveOutputPath(output string) string {
	if strings.HasSuffix(output, ".go") {
		return output
	}
	return filepath.Join(output, "botx_gen.go")
}

type generatorContext struct {
	doc *Doc

	pages      []pageInfo
	errorPage  *pageInfo
	components map[string]schemaInfo
	validators []validatorInfo
	handlers   []handlerInfo
	api        []apiInfo
	i18n       *I18n
	i18nKeys   map[string]struct{}
}

type pageInfo struct {
	Path         string
	Name         string
	Page         Page
	Params       []paramInfo
	PathParams   []paramInfo
	QueryParams  []paramInfo
	MatcherName  string
	MatcherOk    string
	MatcherParam string
}

type paramInfo struct {
	Name           string
	GoName         string
	GetterName     string
	In             string
	GoType         string
	Required       bool
	DefaultLiteral string
}

type fieldInfo struct {
	Name       string
	GoName     string
	GetterName string
	GoType     string
}

type schemaInfo struct {
	Name   string
	Schema *openapi3.Schema
	Fields []fieldInfo
}

type validatorInfo struct {
	Name       string
	MethodName string
}

type handlerInfo struct {
	Match       string
	MatchType   string
	HandlerType string
	MethodName  string
}

type apiInfo struct {
	Name   string
	Args   []paramInfo
	GoName string
}

func newGeneratorContext(doc *Doc) *generatorContext {
	return &generatorContext{doc: doc}
}

func (g *generatorContext) prepare() error {
	if g.doc == nil {
		return fmt.Errorf("doc is nil")
	}
	if len(g.doc.Pages) == 0 {
		return fmt.Errorf("no pages defined")
	}
	if err := g.prepareI18n(); err != nil {
		return err
	}
	if err := g.prepareComponents(); err != nil {
		return err
	}
	if err := g.preparePages(); err != nil {
		return err
	}
	if err := g.prepareValidators(); err != nil {
		return err
	}
	if err := g.prepareHandlers(); err != nil {
		return err
	}
	if err := g.prepareAPI(); err != nil {
		return err
	}
	return nil
}

func (g *generatorContext) prepareI18n() error {
	if g.doc.I18n == nil {
		return nil
	}
	g.i18n = g.doc.I18n
	keys := make(map[string]struct{})
	for key := range g.doc.I18n.Entries {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys[key] = struct{}{}
	}
	if len(keys) != 0 {
		g.i18nKeys = keys
	}
	return nil
}

func (g *generatorContext) prepareComponents() error {
	components := make(map[string]schemaInfo)
	for name, schema := range g.doc.Components.Schemas {
		if schema == nil {
			continue
		}
		info := schemaInfo{
			Name:   name,
			Schema: schema,
		}
		fields, err := schemaFields(schema)
		if err != nil {
			return fmt.Errorf("component %s: %w", name, err)
		}
		info.Fields = fields
		components[name] = info
	}
	g.components = components
	return nil
}

func (g *generatorContext) preparePages() error {
	paths := make([]string, 0, len(g.doc.Pages))
	for path := range g.doc.Pages {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		normalized := normalizePathPattern(path)
		page := g.doc.Pages[path]
		if normalized == "/error" {
			info := pageInfo{
				Path: normalized,
				Name: "Error",
				Page: page,
			}
			g.errorPage = &info
			continue
		}
		name := pageNameFromPath(normalized)
		info := pageInfo{
			Path: normalized,
			Name: name,
			Page: page,
		}
		params, err := parseParameters(page.Parameters)
		if err != nil {
			return fmt.Errorf("page %s: %w", path, err)
		}
		info.Params = params
		for _, param := range params {
			switch strings.ToLower(param.In) {
			case "path":
				info.PathParams = append(info.PathParams, param)
			case "query", "":
				info.QueryParams = append(info.QueryParams, param)
			default:
				info.QueryParams = append(info.QueryParams, param)
			}
		}
		if pathHasParams(path) {
			info.MatcherName = lowerFirst(name) + "Matcher"
			info.MatcherOk = "ok" + name
			info.MatcherParam = "params" + name
		}
		g.pages = append(g.pages, info)
	}
	return nil
}

func (g *generatorContext) prepareValidators() error {
	validators := map[string]validatorInfo{}
	for _, page := range g.pages {
		if page.Page.Form == nil {
			continue
		}
		for _, field := range page.Page.Form.Fields {
			if field.Validator == nil {
				continue
			}
			name := strings.TrimSpace(string(*field.Validator))
			if name == "" {
				continue
			}
			if _, ok := validators[name]; ok {
				continue
			}
			validators[name] = validatorInfo{
				Name:       name,
				MethodName: validatorMethodName(page.Name),
			}
		}
	}
	keys := make([]string, 0, len(validators))
	for key := range validators {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g.validators = append(g.validators, validators[key])
	}
	return nil
}

func (g *generatorContext) prepareHandlers() error {
	for _, handler := range g.doc.Handlers {
		match := strings.TrimSpace(string(handler.Match))
		if match == "" {
			continue
		}
		info := handlerInfo{
			Match:       match,
			MatchType:   strings.ToLower(strings.TrimSpace(handler.MatchType)),
			HandlerType: strings.ToLower(strings.TrimSpace(handler.Type)),
			MethodName:  handlerMethodName(match, handler.Type),
		}
		g.handlers = append(g.handlers, info)
	}
	return nil
}

func (g *generatorContext) prepareAPI() error {
	if len(g.doc.API) == 0 {
		return nil
	}
	keys := make([]string, 0, len(g.doc.API))
	for name := range g.doc.API {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		api := g.doc.API[name]
		info := apiInfo{
			Name:   name,
			GoName: toCamel(name),
		}
		for _, arg := range api.Args {
			if arg == nil {
				continue
			}
			goType := schemaRefToGoType(arg.Schema)
			info.Args = append(info.Args, paramInfo{
				Name:       arg.Name,
				GoName:     goFieldName(arg.Name),
				GetterName: getterName(arg.Name),
				GoType:     goType,
			})
		}
		g.api = append(g.api, info)
	}
	return nil
}

func (g *generatorContext) render() ([]byte, error) {
	buf := &bytes.Buffer{}
	writer := &codeWriter{buf: buf}
	packageName := strings.TrimSpace(g.doc.Package)
	if packageName == "" {
		packageName = "botx"
	}
	writer.line("package %s", packageName)
	writer.line("")
	if err := g.renderImports(writer); err != nil {
		return nil, err
	}
	writer.line("")
	if err := g.renderSchemas(writer); err != nil {
		return nil, err
	}
	if err := g.renderCore(writer); err != nil {
		return nil, err
	}
	if err := g.renderPagesDispatch(writer); err != nil {
		return nil, err
	}
	if err := g.renderParameterParsers(writer); err != nil {
		return nil, err
	}
	if err := g.renderForms(writer); err != nil {
		return nil, err
	}
	if err := g.renderInterfaces(writer); err != nil {
		return nil, err
	}
	if err := g.renderPageRenderer(writer); err != nil {
		return nil, err
	}
	if err := g.renderHelpers(writer); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type codeWriter struct {
	buf *bytes.Buffer
}

func (w *codeWriter) line(format string, args ...any) {
	if len(args) == 0 {
		w.buf.WriteString(format)
		w.buf.WriteByte('\n')
		return
	}
	fmt.Fprintf(w.buf, format, args...)
	w.buf.WriteByte('\n')
}

func renderTemplate(w *codeWriter, name string, tpl string, data any, funcs template.FuncMap) error {
	tmpl := template.New(name)
	if funcs != nil {
		tmpl = tmpl.Funcs(funcs)
	}
	parsed, err := tmpl.Parse(tpl)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", name, err)
	}
	if err := parsed.Execute(w.buf, data); err != nil {
		return fmt.Errorf("execute template %s: %w", name, err)
	}
	return nil
}

type exprContext struct {
	paramExprs     map[string]string
	stateExprs     map[string]string
	itemExprs      map[string]string
	stateItemsExpr string
	errExpr        string
	i18nKeys       map[string]struct{}
	i18nFunc       string
}

func (g *generatorContext) renderImports(w *codeWriter) error {
	w.line("import (")
	w.line("\t\"context\"")
	w.line("\t\"encoding/json\"")
	w.line("\t\"fmt\"")
	w.line("\t\"net/url\"")
	w.line("\t\"strings\"")
	w.line("")
	w.line("\t\"github.com/anclax/botx/pkg/core/bot\"")
	w.line("\t\"github.com/anclax/botx/pkg/core/routepath\"")
	w.line("\t\"github.com/anclax/botx/pkg/core/session\"")
	w.line("\t\"github.com/pkg/errors\"")
	w.line(")")
	return nil
}

func (g *generatorContext) renderSchemas(w *codeWriter) error {
	if len(g.components) == 0 {
		return nil
	}
	w.line("// schemas")
	w.line("")

	keys := make([]string, 0, len(g.components))
	for key := range g.components {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		component := g.components[key]
		w.line("type %s struct {", component.Name)
		for _, field := range component.Fields {
			w.line("\t%s %s", field.GoName, field.GoType)
		}
		w.line("}")
		w.line("")

		if len(component.Fields) != 0 {
			w.line("func New%s(%s) *%s {", component.Name, constructorArgs(component.Fields), component.Name)
			w.line("\treturn &%s{", component.Name)
			for _, field := range component.Fields {
				w.line("\t\t%s: %s,", field.GoName, field.GoName)
			}
			w.line("\t}")
			w.line("}")
			w.line("")
		}

		for _, field := range component.Fields {
			if field.GetterName == "" {
				continue
			}
			w.line("func (v %s) %s() %s {", component.Name, field.GetterName, field.GoType)
			w.line("\treturn v.%s", field.GoName)
			w.line("}")
			w.line("")
		}
	}
	return nil
}

type coreTemplateData struct {
	Handlers   []handlerInfo
	Validators []validatorInfo
}

const coreTemplate = `// Core architecture components

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
{{- range .Handlers }}
	{{ .MethodName }}(ctx context.Context, chatID int64, b *bot.Bot) error
{{- end }}
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
{{- if .Handlers }}
{{- range .Handlers }}
	if {{ handlerCondition . }} && h.commandHandler != nil {
		if err := h.commandHandler.{{ .MethodName }}(ctx, chatID, h.bot); err != nil {
			return errors.Wrap(err, "failed to handle {{ .Match }} command")
		}
		return nil
	}
{{- end }}
	if err := h.defaultHandler.HandleTextMessage(ctx, data, chatID, h.bot); err != nil {
		return errors.Wrap(err, "failed to handle text message in default handler")
	}
	return nil
{{- else }}
	if err := h.defaultHandler.HandleTextMessage(ctx, data, chatID, h.bot); err != nil {
		return errors.Wrap(err, "failed to handle text message in default handler")
	}
	return nil
{{- end }}
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
{{- range .Validators }}
	case {{ printf "%q" .Name }}:
		return h.formValidator.{{ .MethodName }}(ctx, chatID, url, input)
{{- end }}
	default:
		return nil, errors.Wrapf(bot.ErrNotFound, "unknown validator: %s", validator)
	}
}
`

func (g *generatorContext) renderCore(w *codeWriter) error {
	data := coreTemplateData{
		Handlers:   g.handlers,
		Validators: g.validators,
	}
	return renderTemplate(w, "core", coreTemplate, data, template.FuncMap{
		"handlerCondition": handlerCondition,
	})
}

func (g *generatorContext) renderPagesDispatch(w *codeWriter) error {
	w.line("// code generated for pages")
	w.line("")

	paramPages := make([]pageInfo, 0)
	for _, page := range g.pages {
		if len(page.PathParams) != 0 {
			paramPages = append(paramPages, page)
		}
	}
	if len(paramPages) != 0 {
		w.line("var (")
		for _, page := range paramPages {
			w.line("\t%s = routepath.MustCompile(%q)", page.MatcherName, page.Path)
		}
		w.line(")")
		w.line("")
	}

	w.line("func (h *BotxHandler) onRoute(ctx context.Context, chatID int64, url *url.URL) error {")
	for _, page := range paramPages {
		w.line("\t%s, %s := %s.Match(url.Path)", page.MatcherParam, page.MatcherOk, page.MatcherName)
	}
	if len(paramPages) != 0 {
		w.line("")
	}
	w.line("\tswitch {")

	staticPages, matcherPages := splitPagesForDispatch(g.pages)
	for _, page := range staticPages {
		w.line("\tcase url.Path == %q:", page.Path)
		renderRouteCase(w, page)
	}
	for _, page := range matcherPages {
		w.line("\tcase %s:", page.MatcherOk)
		renderRouteCase(w, page)
	}
	w.line("\tdefault:")
	w.line("\t\treturn errors.Wrapf(bot.ErrNotFound, \"unknown route: %s\", url.String())")
	w.line("\t}")
	w.line("\treturn nil")
	w.line("}")
	w.line("")

	w.line("func (h *BotxHandler) onSubmit(ctx context.Context, chatID int64, url *url.URL) error {")
	w.line("\traw := url.Query().Get(\"values\")")
	w.line("\tif raw == \"\" {")
	w.line("\t\treturn errors.Wrap(bot.ErrBadRequest, \"missing form values\")")
	w.line("\t}")
	w.line("\tvar values bot.FormValues")
	w.line("\tif err := json.Unmarshal([]byte(raw), &values); err != nil {")
	w.line("\t\treturn errors.Wrap(err, \"failed to unmarshal form values\")")
	w.line("\t}")
	w.line("")

	formPages := make([]pageInfo, 0)
	for _, page := range g.pages {
		if page.Page.Form == nil {
			continue
		}
		formPages = append(formPages, page)
	}

	formMatcherPages := make([]pageInfo, 0)
	for _, page := range formPages {
		if len(page.PathParams) != 0 {
			formMatcherPages = append(formMatcherPages, page)
			w.line("\t%s, %s := %s.Match(url.Path)", page.MatcherParam, page.MatcherOk, page.MatcherName)
		}
	}
	if len(formMatcherPages) != 0 {
		w.line("")
	}

	w.line("\tswitch {")
	staticForms, matcherForms := splitPagesForDispatch(formPages)
	for _, page := range staticForms {
		w.line("\tcase url.Path == %q:", page.Path)
		renderSubmitCase(w, page)
	}
	for _, page := range matcherForms {
		w.line("\tcase %s:", page.MatcherOk)
		renderSubmitCase(w, page)
	}
	w.line("\tdefault:")
	groupName := formGroupName(formPages)
	if groupName == "" {
		groupName = "form"
	}
	if groupName == "form" {
		w.line("\t\treturn errors.Wrapf(bot.ErrNotFound, \"unknown form: %s\", url.Path)")
	} else {
		w.line("\t\treturn errors.Wrapf(bot.ErrNotFound, \"unknown form %s: %%s\", url.Path)", groupName)
	}
	w.line("\t}")
	w.line("\treturn nil")
	w.line("}")
	return nil
}

func (g *generatorContext) renderParameterParsers(w *codeWriter) error {
	w.line("// url to params")
	w.line("")
	for _, page := range g.pages {
		params := page.Params
		funcName := fmt.Sprintf("ParseParametersPage%s", page.Name)
		signature := "url *url.URL"
		if len(page.PathParams) != 0 {
			if len(page.QueryParams) != 0 {
				signature = "url *url.URL, params routepath.Params"
			} else {
				signature = "params routepath.Params"
			}
		}
		w.line("func %s(%s) (*ParametersPage%s, error) {", funcName, signature, page.Name)
		if len(params) == 0 {
			w.line("\treturn &ParametersPage%s{}, nil", page.Name)
			w.line("}")
			w.line("")
			continue
		}

		for _, param := range params {
			parseParam(w, param, page)
		}
		w.line("\treturn &ParametersPage%s{", page.Name)
		for _, param := range params {
			w.line("\t\t%s: %s,", param.GoName, lowerFirst(param.GoName))
		}
		w.line("\t}, nil")
		w.line("}")
		w.line("")
	}
	return nil
}

func (g *generatorContext) renderForms(w *codeWriter) error {
	if !hasForms(g.pages) {
		return nil
	}
	w.line("// forms")
	w.line("")
	for _, page := range g.pages {
		if page.Page.Form == nil {
			continue
		}
		form := page.Page.Form
		fields := sortedFormFields(form.Fields, form.Required)
		w.line("type Form%s struct {", page.Name)
		for _, field := range fields {
			w.line("\t%s string", field.goName)
		}
		w.line("}")
		w.line("")
		for _, field := range fields {
			w.line("func (f *Form%s) %s() string {", page.Name, getterName(field.name))
			w.line("\treturn f.%s", field.goName)
			w.line("}")
			w.line("")
		}
		w.line("func unmarshalForm%s(values bot.FormValues) (*Form%s, error) {", page.Name, page.Name)
		for _, field := range fields {
			if field.required {
				w.line("\t%s, ok := values[%q]", field.goName, field.name)
				w.line("\tif !ok {")
				w.line("\t\treturn nil, errors.Wrap(bot.ErrBadRequest, %q)", fmt.Sprintf("%s is required", field.name))
				w.line("\t}")
			} else {
				w.line("\t%s := values[%q]", field.goName, field.name)
			}
		}
		w.line("\treturn &Form%s{", page.Name)
		for _, field := range fields {
			w.line("\t\t%s: %s,", field.goName, field.goName)
		}
		w.line("\t}, nil")
		w.line("}")
		w.line("")
	}
	return nil
}

type interfacesTemplateData struct {
	Validators []validatorInfo
	Pages      []stateProviderTemplatePage
}

type stateProviderTemplatePage struct {
	Name    string
	HasForm bool
}

const interfacesTemplate = `// FormValidator

type FormValidator interface {
{{- range .Validators }}
	{{ .MethodName }}(ctx context.Context, chatID int64, url *url.URL, input string) (*bot.ValidateResult, error)
{{- end }}
}

// StateProvider provides state views.
type StateProvider interface {
{{- range .Pages }}
{{- if .HasForm }}
	Provide{{ .Name }}State(ctx context.Context, chatID int64, form *Form{{ .Name }}, parameters *ParametersPage{{ .Name }}) (*StatePage{{ .Name }}, error)
{{- else }}
	Provide{{ .Name }}State(ctx context.Context, chatID int64, parameters *ParametersPage{{ .Name }}) (*StatePage{{ .Name }}, error)
{{- end }}
{{- end }}
}
`

func (g *generatorContext) renderInterfaces(w *codeWriter) error {
	pages := make([]stateProviderTemplatePage, 0, len(g.pages))
	for _, page := range g.pages {
		pages = append(pages, stateProviderTemplatePage{
			Name:    page.Name,
			HasForm: page.Page.Form != nil,
		})
	}
	data := interfacesTemplateData{
		Validators: g.validators,
		Pages:      pages,
	}
	return renderTemplate(w, "interfaces", interfacesTemplate, data, nil)
}

func (g *generatorContext) renderPageRenderer(w *codeWriter) error {
	w.line("type PageRenderer struct {")
	w.line("\tb *bot.Bot")
	w.line("}")
	w.line("")

	for _, page := range g.pages {
		g.renderParametersStruct(w, page)
		g.renderStateStruct(w, page)
		g.renderPageView(w, page)
		if page.Page.Form != nil {
			g.renderFormView(w, page)
		}
	}
	if g.errorPage != nil {
		g.renderErrorPageView(w, *g.errorPage)
	} else {
		g.renderErrorPageView(w, pageInfo{Page: Page{}})
	}
	return nil
}

func (g *generatorContext) renderHelpers(w *codeWriter) error {
	w.line("func cond[T any](condition bool, a, b T) T {")
	w.line("\tif condition {")
	w.line("\t\treturn a")
	w.line("\t}")
	w.line("\treturn b")
	w.line("}")
	w.line("")

	w.line("func appendButtonGrids(grids ...[][]bot.Button) [][]bot.Button {")
	w.line("\tvar result [][]bot.Button")
	w.line("\tfor _, grid := range grids {")
	w.line("\t\tresult = append(result, grid...)")
	w.line("\t}")
	w.line("\treturn result")
	w.line("}")
	w.line("")

	w.line("func forEach[T any](slice []T, fn func(index int, item T) string) string {")
	w.line("\tvar sb strings.Builder")
	w.line("\tfor i, v := range slice {")
	w.line("\t\tsb.WriteString(fn(i, v))")
	w.line("\t}")
	w.line("\treturn sb.String()")
	w.line("}")
	w.line("")

	if g.i18n != nil {
		g.renderI18n(w)
		w.line("")
	}

	if g.doc.Navbar != nil {
		ctx := exprContext{i18nKeys: g.i18nKeys, i18nFunc: "i18nStatic(%q)"}
		w.line("var navbar = %s", buttonRowLiteral(g.doc.Navbar.Rows, ctx))
		w.line("")
	}

	if helpers := paginationHelpers(g.pages); len(helpers) != 0 {
		for _, helper := range helpers {
			g.renderPaginationHelper(w, helper)
			w.line("")
		}
	}

	defaults := defaultConstants(g.pages)
	if len(defaults) != 0 {
		w.line("const (")
		for _, item := range defaults {
			w.line("\t%s = %s", item.name, item.value)
		}
		w.line(")")
		w.line("")
	}

	w.line("func parseOptionalIntQuery(url *url.URL, key string, defaultValue int) (int, error) {")
	w.line("\tvalue := url.Query().Get(key)")
	w.line("\tif value == \"\" {")
	w.line("\t\treturn defaultValue, nil")
	w.line("\t}")
	w.line("\treturn ToInt(value)")
	w.line("}")
	w.line("")

	w.line("func parseOptionalInt64Query(url *url.URL, key string, defaultValue int64) (int64, error) {")
	w.line("\tvalue := url.Query().Get(key)")
	w.line("\tif value == \"\" {")
	w.line("\t\treturn defaultValue, nil")
	w.line("\t}")
	w.line("\treturn ToInt64(value)")
	w.line("}")
	w.line("")

	w.line("func parseOptionalStringQuery(url *url.URL, key string, defaultValue string) string {")
	w.line("\tvalue := url.Query().Get(key)")
	w.line("\tif value == \"\" {")
	w.line("\t\treturn defaultValue")
	w.line("\t}")
	w.line("\treturn value")
	w.line("}")
	w.line("")

	w.line("func parseParamInt64(params routepath.Params, key string) (int64, error) {")
	w.line("\tvalue, ok := params.Get(key)")
	w.line("\tif !ok || value == \"\" {")
	w.line("\t\treturn 0, errors.Wrapf(bot.ErrBadRequest, \"missing %s parameter\", key)")
	w.line("\t}")
	w.line("\treturn ToInt64(value)")
	w.line("}")
	w.line("")

	w.line("func ToInt(s string) (int, error) {")
	w.line("\tvar i int")
	w.line("\t_, err := fmt.Sscanf(s, \"%d\", &i)")
	w.line("\tif err != nil {")
	w.line("\t\treturn 0, err")
	w.line("\t}")
	w.line("\treturn i, nil")
	w.line("}")
	w.line("")

	w.line("func ToInt32(s string) (int32, error) {")
	w.line("\tvar i int32")
	w.line("\t_, err := fmt.Sscanf(s, \"%d\", &i)")
	w.line("\tif err != nil {")
	w.line("\t\treturn 0, err")
	w.line("\t}")
	w.line("\treturn i, nil")
	w.line("}")
	w.line("")

	w.line("func ToInt64(s string) (int64, error) {")
	w.line("\tvar i int64")
	w.line("\t_, err := fmt.Sscanf(s, \"%d\", &i)")
	w.line("\tif err != nil {")
	w.line("\t\treturn 0, err")
	w.line("\t}")
	w.line("\treturn i, nil")
	w.line("}")
	w.line("")

	w.line("func ptr[T any](v T) *T {")
	w.line("\treturn &v")
	w.line("}")
	return nil
}

func (g *generatorContext) renderI18n(w *codeWriter) {
	defaultLang := ""
	if g.i18n != nil {
		defaultLang = strings.TrimSpace(g.i18n.Default)
	}
	if defaultLang == "" {
		defaultLang = firstI18nLanguage(g.i18n)
	}
	if defaultLang == "" {
		defaultLang = "en"
	}
	defaultLang = strings.ToLower(defaultLang)

	w.line("const i18nDefault = %q", defaultLang)
	w.line("")
	w.line("var i18nEntries = map[string]map[string]string{")
	keys := sortedI18nKeys(g.i18n)
	for _, key := range keys {
		entry := g.i18n.Entries[key]
		w.line("\t%q: {", key)
		langs := sortedI18nLangs(entry)
		for _, lang := range langs {
			value := entry[lang]
			w.line("\t\t%q: %q,", strings.ToLower(lang), value)
		}
		w.line("\t},")
	}
	w.line("}")
	w.line("")
	w.line("func i18n(ctx context.Context, _ int64, key string) string {")
	w.line("\tlang := strings.ToLower(bot.LanguageFromContext(ctx))")
	w.line("\tif lang == \"\" {")
	w.line("\t\tlang = i18nDefault")
	w.line("\t}")
	w.line("\tif value := i18nLookup(key, lang); value != \"\" {")
	w.line("\t\treturn value")
	w.line("\t}")
	w.line("\tif lang != i18nDefault {")
	w.line("\t\tif value := i18nLookup(key, i18nDefault); value != \"\" {")
	w.line("\t\t\treturn value")
	w.line("\t\t}")
	w.line("\t}")
	w.line("\treturn i18nFallback(key)")
	w.line("}")
	w.line("")
	w.line("func i18nStatic(key string) string {")
	w.line("\tif value := i18nLookup(key, i18nDefault); value != \"\" {")
	w.line("\t\treturn value")
	w.line("\t}")
	w.line("\treturn i18nFallback(key)")
	w.line("}")
	w.line("")
	w.line("func i18nLookup(key string, lang string) string {")
	w.line("\tentry, ok := i18nEntries[key]")
	w.line("\tif !ok {")
	w.line("\t\treturn \"\"")
	w.line("\t}")
	w.line("\treturn entry[lang]")
	w.line("}")
	w.line("")
	w.line("func i18nFallback(key string) string {")
	w.line("\tentry, ok := i18nEntries[key]")
	w.line("\tif !ok {")
	w.line("\t\treturn key")
	w.line("\t}")
	w.line("\tfor _, value := range entry {")
	w.line("\t\tif value != \"\" {")
	w.line("\t\t\treturn value")
	w.line("\t\t}")
	w.line("\t}")
	w.line("\treturn key")
	w.line("}")
}

func firstI18nLanguage(i18n *I18n) string {
	if i18n == nil {
		return ""
	}
	langs := map[string]struct{}{}
	for _, entry := range i18n.Entries {
		for lang := range entry {
			if strings.TrimSpace(lang) == "" {
				continue
			}
			langs[lang] = struct{}{}
		}
	}
	if len(langs) == 0 {
		return ""
	}
	sorted := make([]string, 0, len(langs))
	for key := range langs {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)
	return sorted[0]
}

func sortedI18nKeys(i18n *I18n) []string {
	if i18n == nil || len(i18n.Entries) == 0 {
		return nil
	}
	keys := make([]string, 0, len(i18n.Entries))
	for key := range i18n.Entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedI18nLangs(entry map[string]string) []string {
	if len(entry) == 0 {
		return nil
	}
	langs := make([]string, 0, len(entry))
	for key := range entry {
		langs = append(langs, key)
	}
	sort.Strings(langs)
	return langs
}

func schemaFields(schema *openapi3.Schema) ([]fieldInfo, error) {
	if schema == nil {
		return nil, nil
	}
	fields := make([]fieldInfo, 0)
	properties := schema.Properties
	ordered := orderedSchemaFields(schema)
	for _, name := range ordered {
		prop := properties[name]
		if prop == nil {
			continue
		}
		goType := schemaRefToGoType(prop)
		exported := goFieldName(name)
		fieldName := exported
		getter := getterName(name)
		if isLowerName(name) {
			if name == "error" {
				fieldName = "errMsg"
			} else {
				fieldName = lowerFirst(exported)
			}
		}
		if unicode.IsUpper(rune(fieldName[0])) {
			getter = ""
		}
		fields = append(fields, fieldInfo{
			Name:       name,
			GoName:     fieldName,
			GetterName: getter,
			GoType:     goType,
		})
	}
	return fields, nil
}

func orderedSchemaFields(schema *openapi3.Schema) []string {
	if schema == nil {
		return nil
	}
	keys := make([]string, 0, len(schema.Properties))
	for key := range schema.Properties {
		keys = append(keys, key)
	}
	required := make([]string, 0)
	requiredSet := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		requiredSet[name] = struct{}{}
		if _, ok := schema.Properties[name]; ok {
			required = append(required, name)
		}
	}
	remaining := make([]string, 0)
	for _, key := range keys {
		if _, ok := requiredSet[key]; ok {
			continue
		}
		remaining = append(remaining, key)
	}
	sort.Strings(remaining)
	return append(required, remaining...)
}

func pageNameFromPath(path string) string {
	path = normalizePathPattern(path)
	if path == "" || path == "/" {
		return "Root"
	}
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return "Root"
	}
	var nameParts []string
	for i, part := range parts {
		if part == "" {
			continue
		}
		if isPathParam(part) {
			if i != len(parts)-1 {
				continue
			}
			param := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			nameParts = append(nameParts, toCamel(param))
			continue
		}
		nameParts = append(nameParts, toCamel(part))
	}
	if len(nameParts) == 0 {
		return "Root"
	}
	return strings.Join(nameParts, "")
}

func parseParameters(params Parameters) ([]paramInfo, error) {
	if len(params) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var result []paramInfo
	for _, key := range keys {
		param := params[key]
		if param == nil {
			continue
		}
		name := param.Name
		if name == "" {
			name = key
		}
		goType := schemaRefToGoType(param.Schema)
		in := strings.ToLower(strings.TrimSpace(param.In))
		if in == "" {
			in = "query"
		}
		required := param.Required
		defaultLiteral := ""
		if param.Schema != nil && param.Schema.Value != nil && param.Schema.Value.Default != nil {
			defaultLiteral = defaultLiteralFor(goType, param.Schema.Value.Default)
		}
		result = append(result, paramInfo{
			Name:           name,
			GoName:         paramFieldName(name),
			GetterName:     getterName(name),
			In:             in,
			GoType:         goType,
			Required:       required,
			DefaultLiteral: defaultLiteral,
		})
	}
	return result, nil
}

func pathHasParams(path string) bool {
	path = normalizePathPattern(path)
	return strings.Contains(path, "{") && strings.Contains(path, "}")
}

func normalizePathPattern(path string) string {
	if path == "" {
		return path
	}
	path = strings.ReplaceAll(path, "${", "{")
	return path
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) == 1 {
		runes[0] = unicode.ToLower(runes[0])
		return string(runes)
	}
	if unicode.IsUpper(runes[0]) && unicode.IsUpper(runes[1]) {
		idx := 1
		for idx < len(runes) && unicode.IsUpper(runes[idx]) {
			idx++
		}
		if idx == len(runes) {
			return strings.ToLower(s)
		}
		for i := 0; i < idx-1; i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
		return string(runes)
	}
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func validatorMethodName(pageName string) string {
	return "ValidateForm" + pageName
}

func handlerMethodName(match string, handlerType string) string {
	name := strings.TrimSpace(match)
	name = strings.TrimPrefix(name, "/")
	return "Handle" + toCamel(handlerType) + toCamel(name)
}

func toCamel(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if isAllUpper(part) {
			result.WriteString(part)
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		result.WriteString(string(runes))
	}
	return result.String()
}

func schemaRefToGoType(schema *openapi3.SchemaRef) string {
	if schema == nil {
		return "any"
	}
	if schema.Ref != "" {
		return toCamel(refName(schema.Ref))
	}
	if schema.Value != nil {
		return schemaToGoType(schema.Value)
	}
	return "any"
}

func schemaToGoType(schema *openapi3.Schema) string {
	if schema == nil {
		return "any"
	}
	if schema.Type == nil || len(*schema.Type) == 0 {
		return "any"
	}
	value := (*schema.Type)[0]
	switch value {
	case "string":
		return "string"
	case "integer":
		switch schema.Format {
		case "int64":
			return "int64"
		case "int32":
			return "int32"
		default:
			return "int"
		}
	case "number":
		switch schema.Format {
		case "float", "float32":
			return "float32"
		default:
			return "float64"
		}
	case "boolean":
		return "bool"
	case "array":
		if schema.Items != nil {
			return "[]" + schemaRefToGoType(schema.Items)
		}
		return "[]any"
	case "object":
		if schema.AdditionalProperties.Has != nil && schema.AdditionalProperties.Schema != nil {
			return "map[string]" + schemaRefToGoType(schema.AdditionalProperties.Schema)
		}
		return "map[string]any"
	default:
		return "any"
	}
}

func goFieldName(name string) string {
	return toCamel(name)
}

func getterName(name string) string {
	return "Get" + goFieldName(name)
}

func refName(ref string) string {
	ref = strings.TrimPrefix(ref, "#/components/schemas/")
	ref = strings.TrimPrefix(ref, "#components/schemas/")
	ref = strings.TrimPrefix(ref, "#/components/")
	ref = strings.TrimPrefix(ref, "#components/")
	ref = strings.TrimPrefix(ref, "#")
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func isPathParam(segment string) bool {
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}")
}

func isAllUpper(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) && unicode.IsLower(r) {
			return false
		}
	}
	return value != ""
}

func isLowerName(name string) bool {
	for _, r := range name {
		return unicode.IsLower(r)
	}
	return false
}

func paramFieldName(name string) string {
	if name == "error" {
		return "errMsg"
	}
	exported := goFieldName(name)
	if isLowerName(name) {
		return lowerFirst(exported)
	}
	return exported
}

func defaultLiteralFor(goType string, value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func constructorArgs(fields []fieldInfo) string {
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf("%s %s", field.GoName, field.GoType))
	}
	return strings.Join(parts, ", ")
}

func handlerCondition(handler handlerInfo) string {
	match := strconv.Quote(handler.Match)
	switch handler.MatchType {
	case "prefix":
		return fmt.Sprintf("strings.HasPrefix(data, %s)", match)
	default:
		return fmt.Sprintf("data == %s", match)
	}
}

func splitPagesForDispatch(pages []pageInfo) ([]pageInfo, []pageInfo) {
	staticPages := make([]pageInfo, 0)
	matcherPages := make([]pageInfo, 0)
	for _, page := range pages {
		if len(page.PathParams) == 0 {
			staticPages = append(staticPages, page)
		} else {
			matcherPages = append(matcherPages, page)
		}
	}
	sort.SliceStable(matcherPages, func(i, j int) bool {
		return len(matcherPages[i].Path) > len(matcherPages[j].Path)
	})
	return staticPages, matcherPages
}

func renderRouteCase(w *codeWriter, page pageInfo) {
	parseCall := parseParametersCall(page)
	if page.Page.Form != nil {
		w.line("\t\tparams, err := %s", parseCall)
		w.line("\t\tif err != nil {")
		w.line("\t\t\treturn errors.Wrap(err, \"invalid parameters for page %s\")", page.Path)
		w.line("\t\t}")
		w.line("\t\tif err := h.renderer.form%s(ctx, chatID, url, params); err != nil {", page.Name)
		w.line("\t\t\treturn errors.Wrap(err, \"failed to render form for page %s\")", page.Path)
		w.line("\t\t}")
		return
	}
	w.line("\t\tparams, err := %s", parseCall)
	w.line("\t\tif err != nil {")
	w.line("\t\t\treturn errors.Wrap(err, \"invalid parameters for page %s\")", page.Path)
	w.line("\t\t}")
	w.line("\t\tstate, err := h.sp.Provide%sState(ctx, chatID, params)", page.Name)
	w.line("\t\tif err != nil {")
	w.line("\t\t\treturn errors.Wrap(err, \"failed to provide state for page %s\")", page.Path)
	w.line("\t\t}")
	w.line("\t\tif err := h.renderer.page%s(ctx, chatID, state, params); err != nil {", page.Name)
	w.line("\t\t\treturn errors.Wrap(err, \"failed to render page %s\")", page.Path)
	w.line("\t\t}")
}

func renderSubmitCase(w *codeWriter, page pageInfo) {
	parseCall := parseParametersCall(page)
	w.line("\t\tparams, err := %s", parseCall)
	w.line("\t\tif err != nil {")
	w.line("\t\t\treturn errors.Wrap(err, \"invalid parameters for form %s\")", page.Path)
	w.line("\t\t}")
	w.line("\t\tform, err := unmarshalForm%s(values)", page.Name)
	w.line("\t\tif err != nil {")
	w.line("\t\t\treturn errors.Wrap(err, \"failed to unmarshal form for %s\")", page.Path)
	w.line("\t\t}")
	w.line("\t\tstate, err := h.sp.Provide%sState(ctx, chatID, form, params)", page.Name)
	w.line("\t\tif err != nil {")
	w.line("\t\t\treturn errors.Wrap(err, \"failed to provide state for form %s\")", page.Path)
	w.line("\t\t}")
	w.line("\t\tif err := h.renderer.page%s(ctx, chatID, state, params); err != nil {", page.Name)
	w.line("\t\t\treturn errors.Wrap(err, \"failed to render page for form %s\")", page.Path)
	w.line("\t\t}")
}

func parseParametersCall(page pageInfo) string {
	funcName := fmt.Sprintf("ParseParametersPage%s", page.Name)
	if len(page.PathParams) != 0 {
		if len(page.QueryParams) != 0 {
			return fmt.Sprintf("%s(url, %s)", funcName, page.MatcherParam)
		}
		return fmt.Sprintf("%s(%s)", funcName, page.MatcherParam)
	}
	return fmt.Sprintf("%s(url)", funcName)
}

func parseParam(w *codeWriter, param paramInfo, page pageInfo) {
	paramName := strings.ToLower(param.Name)
	goVar := lowerFirst(param.GoName)
	switch strings.ToLower(param.In) {
	case "path":
		switch param.GoType {
		case "int64":
			w.line("\t%s, err := parseParamInt64(params, %q)", goVar, param.Name)
			w.line("\tif err != nil {")
			w.line("\t\treturn nil, errors.Wrapf(bot.ErrBadRequest, \"invalid %s parameter: %%s\", err.Error())", paramName)
			w.line("\t}")
		default:
			w.line("\t%s, ok := params.Get(%q)", goVar, param.Name)
			w.line("\tif !ok || %s == \"\" {", goVar)
			w.line("\t\treturn nil, errors.Wrap(bot.ErrBadRequest, %q)", fmt.Sprintf("missing %s parameter", paramName))
			w.line("\t}")
		}
	default:
		switch param.GoType {
		case "int":
			if !param.Required {
				defaultValue := param.DefaultLiteral
				if defaultValue == "" {
					defaultValue = "0"
				} else {
					defaultValue = defaultConstName(page.Name, param.Name, param.DefaultLiteral)
				}
				w.line("\t%s, err := parseOptionalIntQuery(url, %q, %s)", goVar, param.Name, defaultValue)
				w.line("\tif err != nil {")
				w.line("\t\treturn nil, errors.Wrapf(bot.ErrBadRequest, \"invalid %s parameter: %%s\", err.Error())", paramName)
				w.line("\t}")
				break
			}
			w.line("\t%sValue := url.Query().Get(%q)", goVar, param.Name)
			w.line("\tif %sValue == \"\" {", goVar)
			w.line("\t\treturn nil, errors.Wrap(bot.ErrBadRequest, %q)", fmt.Sprintf("missing %s parameter", paramName))
			w.line("\t}")
			w.line("\t%s, err := ToInt(%sValue)", goVar, goVar)
			w.line("\tif err != nil {")
			w.line("\t\treturn nil, errors.Wrapf(bot.ErrBadRequest, \"invalid %s parameter: %%s\", err.Error())", paramName)
			w.line("\t}")
		case "int64":
			if !param.Required {
				defaultValue := param.DefaultLiteral
				if defaultValue == "" {
					defaultValue = "0"
				} else {
					defaultValue = defaultConstName(page.Name, param.Name, param.DefaultLiteral)
				}
				w.line("\t%s, err := parseOptionalInt64Query(url, %q, %s)", goVar, param.Name, defaultValue)
				w.line("\tif err != nil {")
				w.line("\t\treturn nil, errors.Wrapf(bot.ErrBadRequest, \"invalid %s parameter: %%s\", err.Error())", paramName)
				w.line("\t}")
				break
			}
			w.line("\t%sValue := url.Query().Get(%q)", goVar, param.Name)
			w.line("\tif %sValue == \"\" {", goVar)
			w.line("\t\treturn nil, errors.Wrap(bot.ErrBadRequest, %q)", fmt.Sprintf("missing %s parameter", paramName))
			w.line("\t}")
			w.line("\t%s, err := ToInt64(%sValue)", goVar, goVar)
			w.line("\tif err != nil {")
			w.line("\t\treturn nil, errors.Wrapf(bot.ErrBadRequest, \"invalid %s parameter: %%s\", err.Error())", paramName)
			w.line("\t}")
		default:
			if !param.Required {
				if param.DefaultLiteral != "" {
					w.line("\t%s := parseOptionalStringQuery(url, %q, %s)", goVar, param.Name, param.DefaultLiteral)
					break
				}
				w.line("\t%s := url.Query().Get(%q)", goVar, param.Name)
				break
			}
			w.line("\t%s := url.Query().Get(%q)", goVar, param.Name)
			w.line("\tif %s == \"\" {", goVar)
			w.line("\t\treturn nil, errors.Wrap(bot.ErrBadRequest, %q)", fmt.Sprintf("missing %s parameter", paramName))
			w.line("\t}")
		}
	}
}

type formFieldInfo struct {
	name     string
	goName   string
	required bool
}

func sortedFormFields(fields map[string]FormField, required []string) []formFieldInfo {
	if len(fields) == 0 {
		return nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	requiredSet := make(map[string]struct{}, len(required))
	for _, name := range required {
		requiredSet[name] = struct{}{}
	}
	result := make([]formFieldInfo, 0, len(keys))
	for _, key := range keys {
		_, ok := requiredSet[key]
		result = append(result, formFieldInfo{
			name:     key,
			goName:   lowerFirst(goFieldName(key)),
			required: ok,
		})
	}
	return result
}

func hasForms(pages []pageInfo) bool {
	for _, page := range pages {
		if page.Page.Form != nil {
			return true
		}
	}
	return false
}

func hasRootPage(pages []pageInfo) bool {
	for _, page := range pages {
		if page.Path == "/" {
			return true
		}
	}
	return false
}

func (g *generatorContext) renderParametersStruct(w *codeWriter, page pageInfo) {
	w.line("type ParametersPage%s struct {", page.Name)
	for _, param := range page.Params {
		w.line("\t%s %s", param.GoName, param.GoType)
	}
	w.line("}")
	w.line("")
	for _, param := range page.Params {
		w.line("func (p *ParametersPage%s) %s() %s {", page.Name, param.GetterName, param.GoType)
		w.line("\treturn p.%s", param.GoName)
		w.line("}")
		w.line("")
	}
}

func (g *generatorContext) renderStateStruct(w *codeWriter, page pageInfo) {
	stateInfo := g.stateInfo(page)
	w.line("type StatePage%s struct {", page.Name)
	for _, field := range stateInfo.fields {
		w.line("\t%s %s", field.GoName, field.GoType)
	}
	w.line("}")
	w.line("")

	if len(stateInfo.fields) != 0 {
		w.line("func NewStatePage%s(%s) *StatePage%s {", page.Name, constructorArgs(stateInfo.fields), page.Name)
		w.line("\treturn &StatePage%s{", page.Name)
		for _, field := range stateInfo.fields {
			w.line("\t\t%s: %s,", field.GoName, field.GoName)
		}
		w.line("\t}")
		w.line("}")
		w.line("")
	}

	for _, getter := range stateInfo.getters {
		w.line("func (s *StatePage%s) %s() %s {", page.Name, getter.name, getter.goType)
		w.line("\treturn %s", getter.expr)
		w.line("}")
		w.line("")
	}
}

type stateGetterInfo struct {
	name   string
	goType string
	expr   string
}

type stateStructInfo struct {
	fields  []fieldInfo
	getters []stateGetterInfo
	itemRef string
}

func (g *generatorContext) stateInfo(page pageInfo) stateStructInfo {
	if page.Page.State == nil {
		return stateStructInfo{}
	}
	if refName, ok := schemaComponentRef(page.Page.State); ok {
		component := g.components[refName]
		fieldName := lowerFirst(component.Name)
		fields := []fieldInfo{{
			Name:   component.Name,
			GoName: fieldName,
			GoType: component.Name,
		}}
		getters := make([]stateGetterInfo, 0, len(component.Fields))
		for _, field := range component.Fields {
			getter := getterName(field.Name)
			expr := fmt.Sprintf("s.%s.%s", fieldName, field.GoName)
			if field.GetterName != "" {
				expr = fmt.Sprintf("s.%s.%s()", fieldName, field.GetterName)
			}
			getters = append(getters, stateGetterInfo{
				name:   getter,
				goType: field.GoType,
				expr:   expr,
			})
		}
		return stateStructInfo{fields: fields, getters: getters}
	}

	fields, _ := schemaFields(page.Page.State)
	getters := make([]stateGetterInfo, 0, len(fields))
	for _, field := range fields {
		getter := getterName(field.Name)
		getters = append(getters, stateGetterInfo{
			name:   getter,
			goType: field.GoType,
			expr:   fmt.Sprintf("s.%s", field.GoName),
		})
	}
	return stateStructInfo{fields: fields, getters: getters}
}

func schemaComponentRef(schema *openapi3.Schema) (string, bool) {
	if schema == nil {
		return "", false
	}
	if len(schema.AllOf) == 1 && schema.AllOf[0] != nil && schema.AllOf[0].Ref != "" {
		return refName(schema.AllOf[0].Ref), true
	}
	if len(schema.OneOf) == 1 && schema.OneOf[0] != nil && schema.OneOf[0].Ref != "" {
		return refName(schema.OneOf[0].Ref), true
	}
	if len(schema.AnyOf) == 1 && schema.AnyOf[0] != nil && schema.AnyOf[0].Ref != "" {
		return refName(schema.AnyOf[0].Ref), true
	}
	return "", false
}

func (g *generatorContext) renderPageView(w *codeWriter, page pageInfo) {
	ctx := g.pageExprContext(page, paginationItemType(page))
	w.line("func (p *PageRenderer) page%s(ctx context.Context, chatID int64, state *StatePage%s, parameters *ParametersPage%s) error {", page.Name, page.Name, page.Name)
	w.line("\tif err := p.b.SendMessage(ctx, chatID, &bot.Message{")
	if page.Page.View.Message != nil {
		w.line("\t\tText: %s,", stringExprToGo(*page.Page.View.Message, ctx))
	} else {
		w.line("\t\tText: \"\",")
	}
	if page.Page.View.ParseMode != nil && *page.Page.View.ParseMode != "" {
		w.line("\t\tParseMode: %q,", string(*page.Page.View.ParseMode))
	}
	g.renderButtons(w, page, ctx)
	w.line("\t}); err != nil {")
	w.line("\t\treturn errors.Wrap(err, \"failed to send page view message page%s\")", page.Name)
	w.line("\t}")
	w.line("\treturn nil")
	w.line("}")
	w.line("")
}

func (g *generatorContext) renderFormView(w *codeWriter, page pageInfo) {
	ctx := g.pageExprContext(page, "")
	form := page.Page.Form
	fields := sortedFormFields(form.Fields, form.Required)
	if len(fields) == 0 {
		return
	}
	w.line("func (p *PageRenderer) form%s(ctx context.Context, chatID int64, url *url.URL, parameters *ParametersPage%s) error {", page.Name, page.Name)
	w.line("\tform := &bot.Form{")
	w.line("\t\tURL: url,")
	w.line("\t\tIdx: 0,")
	w.line("\t\tFields: []bot.FormField{")
	for _, field := range fields {
		definition := form.Fields[field.name]
		w.line("\t\t\t{")
		w.line("\t\t\t\tID:    %q,", field.name)
		if definition.Label != "" {
			w.line("\t\t\t\tLabel: %s,", stringExprToGo(definition.Label, ctx))
		}
		if definition.Input != nil {
			w.line("\t\t\t\tInput: &bot.FormFieldInput{")
			w.line("\t\t\t\t\tSchema: &bot.FormSchema{")
			w.line("\t\t\t\t\t\tType:   %q,", definition.Input.Type)
			w.line("\t\t\t\t\t\tFormat: %q,", definition.Input.Format)
			w.line("\t\t\t\t\t},")
			if definition.Input.Tip != "" {
				w.line("\t\t\t\t\tTip: %s,", stringExprToGo(definition.Input.Tip, ctx))
			}
			w.line("\t\t\t\t},")
		}
		if definition.Validator != nil {
			w.line("\t\t\t\tValidator: ptr(%q),", strings.TrimSpace(string(*definition.Validator)))
		}
		w.line("\t\t\t},")
	}
	w.line("\t\t},")
	w.line("\t}")
	w.line("\tif err := p.b.SendForm(ctx, chatID, form); err != nil {")
	w.line("\t\treturn errors.Wrap(err, \"failed to send form%s\")", page.Name)
	w.line("\t}")
	w.line("\treturn nil")
	w.line("}")
	w.line("")
}

func (g *generatorContext) renderErrorPageView(w *codeWriter, page pageInfo) {
	ctx := exprContext{errExpr: "err.Error()", i18nKeys: g.i18nKeys, i18nFunc: "i18n(ctx, chatID, %q)"}
	w.line("func (p *PageRenderer) pageError(ctx context.Context, chatID int64, err error) error {")
	w.line("\tif err := p.b.SendMessage(ctx, chatID, &bot.Message{")
	if page.Page.View.Message != nil {
		w.line("\t\tText: %s,", stringExprToGo(*page.Page.View.Message, ctx))
	} else {
		w.line("\t\tText: fmt.Sprintf(%q, err.Error()),", "%s")
	}
	if page.Page.View.Buttons != nil {
		g.renderButtons(w, page, ctx)
	}
	w.line("\t}); err != nil {")
	w.line("\t\treturn errors.Wrap(err, \"failed to send page view message pageError\")")
	w.line("\t}")
	w.line("\treturn nil")
	w.line("}")
	w.line("")
}

func (g *generatorContext) renderButtons(w *codeWriter, page pageInfo, ctx exprContext) {
	buttons := page.Page.View.Buttons
	if buttons == nil {
		if g.doc.Navbar == nil {
			return
		}
		grids := []ButtonGrider{ButtonGrid{Rows: g.doc.Navbar.Rows}}
		lines, _ := g.buttonGridsExpr(grids, page, ctx)
		writeExpressionLines(w, "\t\t", "ButtonGrid: ", lines)
		return
	}
	grids := collectButtonGrids(buttons, g.doc.Navbar)
	if len(grids) == 0 {
		return
	}
	lines, _ := g.buttonGridsExpr(grids, page, ctx)
	writeExpressionLines(w, "\t\t", "ButtonGrid: ", lines)
}

func (g *generatorContext) buttonGridsExpr(grids []ButtonGrider, page pageInfo, ctx exprContext) ([]string, error) {
	if len(grids) == 1 {
		return g.buttonGridExpr(grids[0], page, ctx)
	}
	exprs := make([][]string, 0, len(grids))
	for _, grid := range grids {
		lines, err := g.buttonGridExpr(grid, page, ctx)
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, lines)
	}
	return appendExprLines(exprs), nil
}

func (g *generatorContext) buttonGridExpr(grid ButtonGrider, page pageInfo, ctx exprContext) ([]string, error) {
	switch value := grid.(type) {
	case ButtonGrid:
		return buttonGridLines(value, ctx), nil
	case Pagination:
		return g.paginationLines(value, page, ctx)
	default:
		return []string{"[][]bot.Button{}"}, nil
	}
}

func buttonGridLines(grid ButtonGrid, ctx exprContext) []string {
	lines := []string{"[][]bot.Button{"}
	for _, row := range grid.Rows {
		lines = append(lines, "\t{")
		for _, button := range row.Columns {
			label := stringExprToGo(button.Label, ctx)
			onClick := stringExprToGo(button.OnClick, ctx)
			lines = append(lines, fmt.Sprintf("\t\t{Label: %s, CallbackData: bot.CallbackData(%s)},", label, onClick))
		}
		lines = append(lines, "\t},")
	}
	lines = append(lines, "}")
	return lines
}

func appendExprLines(exprs [][]string) []string {
	lines := []string{"appendButtonGrids("}
	for _, expr := range exprs {
		if len(expr) == 0 {
			continue
		}
		for i, line := range expr {
			prefix := "\t"
			if i == 0 {
				lines = append(lines, prefix+line)
				continue
			}
			lines = append(lines, prefix+line)
		}
		lines[len(lines)-1] = lines[len(lines)-1] + ","
	}
	lines = append(lines, ")")
	return lines
}

func (g *generatorContext) paginationLines(pagination Pagination, page pageInfo, ctx exprContext) ([]string, error) {
	funcName := paginationHelperName(g.pages)
	itemType := paginationItemType(page)
	itemCtx := g.pageExprContext(page, itemType)
	lines := []string{fmt.Sprintf("%s(", funcName)}
	lines = append(lines, fmt.Sprintf("\t%s,", codeExprToGo(pagination.Columns, ctx)))
	lines = append(lines, fmt.Sprintf("\t%s,", codeExprToGo(pagination.Rows, ctx)))
	lines = append(lines, fmt.Sprintf("\t%s,", codeExprToGo(pagination.Total, ctx)))
	lines = append(lines, fmt.Sprintf("\t%s,", codeExprToGo(pagination.Page, ctx)))
	lines = append(lines, fmt.Sprintf("\t%s,", codeExprToGo(pagination.Items, ctx)))
	lines = append(lines, fmt.Sprintf("\tfunc(item %s) bot.Button {", itemType))
	lines = append(lines, "\t\treturn bot.Button{")
	lines = append(lines, fmt.Sprintf("\t\t\tLabel: %s,", stringExprToGo(pagination.Item.Label, itemCtx)))
	lines = append(lines, fmt.Sprintf("\t\t\tCallbackData: bot.CallbackData(%s),", stringExprToGo(pagination.Item.OnClick, itemCtx)))
	lines = append(lines, "\t\t}")
	lines = append(lines, "\t},")
	if pagination.PrevLabel != "" {
		lines = append(lines, fmt.Sprintf("\t%s,", stringExprToGo(pagination.PrevLabel, ctx)))
	}
	if pagination.NextLabel != "" {
		lines = append(lines, fmt.Sprintf("\t%s,", stringExprToGo(pagination.NextLabel, ctx)))
	}
	lines = append(lines, ")")
	return lines, nil
}

func paginationItemType(page pageInfo) string {
	if page.Page.State == nil {
		return "any"
	}
	if refName, ok := schemaComponentRef(page.Page.State); ok {
		return refName
	}
	if page.Page.State.Properties != nil {
		if prop, ok := page.Page.State.Properties["items"]; ok && prop != nil {
			goType := schemaRefToGoType(prop)
			return strings.TrimPrefix(goType, "[]")
		}
	}
	return "any"
}

func paginationHelpers(pages []pageInfo) []paginationHelperInfo {
	seen := make(map[string]struct{})
	var helpers []paginationHelperInfo
	for _, page := range pages {
		if page.Page.View.Buttons == nil {
			continue
		}
		if hasPagination(page.Page.View.Buttons) {
			name := paginationHelperName(pages)
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			helpers = append(helpers, paginationHelperInfo{name: name, page: page})
		}
	}
	return helpers
}

type paginationHelperInfo struct {
	name string
	page pageInfo
}

func (g *generatorContext) renderPaginationHelper(w *codeWriter, helper paginationHelperInfo) {
	path := helper.page.Path
	rowKey := paginationParamKey(helper.page, "row")
	colKey := paginationParamKey(helper.page, "column")
	pageKey := paginationParamKey(helper.page, "page")

	w.line("func %s[T any](columns int, rows int, total int, page int, items []T, castFunc func(item T) bot.Button, prevLabel string, nextLabel string) [][]bot.Button {", helper.name)
	w.line("\tvar grid [][]bot.Button")
	w.line("\tif rows <= 0 || columns <= 0 {")
	w.line("\t\treturn grid")
	w.line("\t}")
	w.line("")
	w.line("\tpageSize := rows * columns")
	w.line("\tisLastPage := (page+1)*pageSize >= total")
	w.line("")
	w.line("\tfor r := 0; r < rows; r++ {")
	w.line("\t\tvar row []bot.Button")
	w.line("\t\tfor c := 0; c < columns; c++ {")
	w.line("\t\t\tindex := r*columns + c")
	w.line("\t\t\tif index >= len(items) {")
	w.line("\t\t\t\tbreak")
	w.line("\t\t\t}")
	w.line("\t\t\trow = append(row, castFunc(items[index]))")
	w.line("\t\t}")
	w.line("\t\tif len(row) == 0 {")
	w.line("\t\t\tbreak")
	w.line("\t\t}")
	w.line("\t\tgrid = append(grid, row)")
	w.line("\t}")
	w.line("")
	w.line("\tctrlRow := []bot.Button{}")
	w.line("")
	w.line("\tif page != 0 {")
	w.line("\t\tctrlRow = append(ctrlRow, bot.Button{")
	w.line("\t\t\tLabel:        prevLabel,")
	w.line("\t\t\tCallbackData: bot.CallbackData(fmt.Sprintf(%q, rows, columns, page-1)),", fmt.Sprintf("%s?%s=%%d&%s=%%d&%s=%%d", path, rowKey, colKey, pageKey))
	w.line("\t\t})")
	w.line("\t}")
	w.line("")
	w.line("\tif !isLastPage {")
	w.line("\t\tctrlRow = append(ctrlRow, bot.Button{")
	w.line("\t\t\tLabel:        nextLabel,")
	w.line("\t\t\tCallbackData: bot.CallbackData(fmt.Sprintf(%q, rows, columns, page+1)),", fmt.Sprintf("%s?%s=%%d&%s=%%d&%s=%%d", path, rowKey, colKey, pageKey))
	w.line("\t\t})")
	w.line("\t}")
	w.line("")
	w.line("\tif len(ctrlRow) != 0 {")
	w.line("\t\tgrid = append(grid, ctrlRow)")
	w.line("\t}")
	w.line("")
	w.line("\treturn grid")
	w.line("}")
}

func hasPagination(buttons *Buttons) bool {
	if buttons == nil {
		return false
	}
	if buttons.Pagination != nil {
		return true
	}
	for _, grid := range buttons.Grids {
		if _, ok := grid.(Pagination); ok {
			return true
		}
	}
	return false
}

func paginationHelperName(pages []pageInfo) string {
	count := 0
	var pageName string
	for _, page := range pages {
		if page.Page.View.Buttons != nil && hasPagination(page.Page.View.Buttons) {
			count++
			pageName = page.Name
		}
	}
	if count <= 1 {
		return "pagination"
	}
	return "pagination" + pageName
}

func paginationParamKey(page pageInfo, fallback string) string {
	for _, param := range page.QueryParams {
		if strings.EqualFold(param.Name, fallback) {
			return param.Name
		}
	}
	return fallback
}

func collectButtonGrids(buttons *Buttons, navbar *Navbar) []ButtonGrider {
	if buttons == nil {
		return nil
	}
	var grids []ButtonGrider
	if buttons.Grid != nil {
		grids = append(grids, buttons.Grid)
	}
	if buttons.Pagination != nil {
		grids = append(grids, buttons.Pagination)
	}
	if len(buttons.Grids) != 0 {
		grids = append(grids, buttons.Grids...)
	}
	if navbar != nil {
		grids = append(grids, ButtonGrid{Rows: navbar.Rows})
	}
	return grids
}

func writeExpressionLines(w *codeWriter, indent string, prefix string, lines []string) {
	if len(lines) == 0 {
		return
	}
	if len(lines) == 1 {
		w.line("%s", indent+prefix+lines[0]+",")
		return
	}
	w.line("%s", indent+prefix+lines[0])
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if i == len(lines)-1 {
			w.line("%s", indent+line+",")
			continue
		}
		w.line("%s", indent+line)
	}
}

func buttonRowLiteral(rows []ButtonGridRow, ctx exprContext) string {
	if len(rows) == 0 {
		return "[]bot.Button{}"
	}
	row := rows[0]
	parts := make([]string, 0, len(row.Columns))
	for _, button := range row.Columns {
		label := stringExprToGo(button.Label, ctx)
		onClick := stringExprToGo(button.OnClick, ctx)
		parts = append(parts, fmt.Sprintf("{Label: %s, CallbackData: bot.CallbackData(%s)}", label, onClick))
	}
	return fmt.Sprintf("[]bot.Button{%s}", strings.Join(parts, ", "))
}

func (g *generatorContext) pageExprContext(page pageInfo, itemType string) exprContext {
	paramExprs := make(map[string]string)
	for _, param := range page.Params {
		paramExprs[param.Name] = fmt.Sprintf("parameters.%s()", param.GetterName)
	}

	stateExprs := make(map[string]string)
	if page.Page.State != nil {
		if refName, ok := schemaComponentRef(page.Page.State); ok {
			if component, ok := g.components[refName]; ok {
				for _, field := range component.Fields {
					stateExprs[field.Name] = fmt.Sprintf("state.%s()", getterName(field.Name))
				}
			}
		} else {
			fields, _ := schemaFields(page.Page.State)
			for _, field := range fields {
				stateExprs[field.Name] = fmt.Sprintf("state.%s()", getterName(field.Name))
			}
		}
	}

	itemExprs := make(map[string]string)
	if itemType != "" {
		if component, ok := g.components[itemType]; ok {
			for _, field := range component.Fields {
				if field.GetterName != "" {
					itemExprs[field.Name] = fmt.Sprintf("item.%s()", field.GetterName)
					continue
				}
				itemExprs[field.Name] = fmt.Sprintf("item.%s", field.GoName)
			}
		}
	}

	stateItemsExpr := ""
	if page.Page.State != nil {
		if prop, ok := page.Page.State.Properties["items"]; ok && prop != nil {
			if getter, ok := stateExprs["items"]; ok {
				stateItemsExpr = getter
			}
		}
	}

	return exprContext{
		paramExprs:     paramExprs,
		stateExprs:     stateExprs,
		itemExprs:      itemExprs,
		stateItemsExpr: stateItemsExpr,
		i18nKeys:       g.i18nKeys,
		i18nFunc:       "i18n(ctx, chatID, %q)",
	}
}

func stringExprToGo(expr StringExpr, ctx exprContext) string {
	raw := string(expr)
	if raw == "" {
		return strconv.Quote("")
	}
	if !strings.Contains(raw, "${") {
		return strconv.Quote(raw)
	}

	segments := parseStringExpr(raw)
	var format strings.Builder
	var args []string
	for _, segment := range segments {
		if segment.isExpr {
			args = append(args, rewriteExpr(segment.value, ctx))
			format.WriteString("%v")
			continue
		}
		format.WriteString(escapeFormatLiteral(segment.value))
	}
	formatString := strconv.Quote(format.String())
	if len(args) == 0 {
		return formatString
	}
	return fmt.Sprintf("fmt.Sprintf(%s, %s)", formatString, strings.Join(args, ", "))
}

func codeExprToGo(code Code, ctx exprContext) string {
	value := strings.TrimSpace(string(code))
	if value == "" {
		return ""
	}
	return rewriteExpr(value, ctx)
}

type stringSegment struct {
	isExpr bool
	value  string
}

func parseStringExpr(input string) []stringSegment {
	var segments []stringSegment
	for len(input) > 0 {
		start := strings.Index(input, "${")
		if start == -1 {
			segments = append(segments, stringSegment{value: input})
			break
		}
		if start > 0 {
			segments = append(segments, stringSegment{value: input[:start]})
		}
		rest := input[start+2:]
		end := findExprEnd(rest)
		if end == -1 {
			segments = append(segments, stringSegment{value: input[start:]})
			break
		}
		exprValue := strings.TrimSpace(rest[:end])
		exprValue = strings.TrimSpace(strings.TrimSuffix(exprValue, ","))
		segments = append(segments, stringSegment{isExpr: true, value: exprValue})
		input = rest[end+1:]
	}
	return segments
}

func findExprEnd(input string) int {
	var depth int
	var quote byte
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if quote == '`' {
				if ch == '`' {
					quote = 0
				}
				continue
			}
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '"', '\'', '`':
			quote = ch
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

func escapeFormatLiteral(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	return value
}

func rewriteExpr(expr string, ctx exprContext) string {
	var sb strings.Builder
	for i := 0; i < len(expr); {
		ch := expr[i]
		if isIdentStart(ch) {
			start := i
			i++
			for i < len(expr) && isIdentPart(expr[i]) {
				i++
			}
			ident := expr[start:i]
			if ctx.i18nFunc != "" {
				if replacement, next := i18nExprReplacement(expr, ident, start, i, ctx); replacement != "" {
					sb.WriteString(replacement)
					i = next
					continue
				}
			}
			if ident == "parameters" || ident == "state" || ident == "item" {
				if i < len(expr) && expr[i] == '.' {
					fieldStart := i + 1
					fieldEnd := fieldStart
					for fieldEnd < len(expr) && isIdentPart(expr[fieldEnd]) {
						fieldEnd++
					}
					field := expr[fieldStart:fieldEnd]
					replacement := ""
					switch ident {
					case "parameters":
						replacement = ctx.paramExprs[field]
					case "state":
						replacement = ctx.stateExprs[field]
					case "item":
						replacement = ctx.itemExprs[field]
					}
					if replacement != "" {
						sb.WriteString(replacement)
						i = fieldEnd
						continue
					}
					sb.WriteString(expr[start:fieldEnd])
					i = fieldEnd
					continue
				}
				if ident == "state" && ctx.stateItemsExpr != "" {
					sb.WriteString(ctx.stateItemsExpr)
					continue
				}
			}
			if ident == "err" && ctx.errExpr != "" {
				sb.WriteString(ctx.errExpr)
				continue
			}
			sb.WriteString(ident)
			continue
		}
		sb.WriteByte(ch)
		i++
	}
	return sb.String()
}

func i18nExprReplacement(expr string, ident string, start int, end int, ctx exprContext) (string, int) {
	if ctx.i18nKeys == nil {
		return "", 0
	}
	if _, ok := ctx.i18nKeys[ident]; ok {
		return fmt.Sprintf(ctx.i18nFunc, ident), end
	}
	if end >= len(expr) || expr[end] != '.' {
		return "", 0
	}
	current := ident
	longest := ""
	longestEnd := -1
	idx := end
	for idx < len(expr) && expr[idx] == '.' {
		segStart := idx + 1
		if segStart >= len(expr) || !isIdentStart(expr[segStart]) {
			break
		}
		segEnd := segStart + 1
		for segEnd < len(expr) && isIdentPart(expr[segEnd]) {
			segEnd++
		}
		current = current + "." + expr[segStart:segEnd]
		if _, ok := ctx.i18nKeys[current]; ok {
			longest = current
			longestEnd = segEnd
		}
		idx = segEnd
	}
	if longest == "" {
		return "", 0
	}
	return fmt.Sprintf(ctx.i18nFunc, longest), longestEnd
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func defaultConstName(pageName string, paramName string, defaultLiteral string) string {
	if defaultLiteral == "" {
		switch strings.ToLower(paramName) {
		case "row":
			return "default" + pageName + "Rows"
		case "column":
			return "default" + pageName + "Columns"
		case "page":
			return "default" + pageName + "Page"
		}
		return "default" + pageName + goFieldName(paramName)
	}
	switch strings.ToLower(paramName) {
	case "row":
		return "default" + pageName + "Rows"
	case "column":
		return "default" + pageName + "Columns"
	case "page":
		return "default" + pageName + "Page"
	}
	return "default" + pageName + goFieldName(paramName)
}

type defaultConstant struct {
	name  string
	value string
}

func defaultConstants(pages []pageInfo) []defaultConstant {
	seen := make(map[string]struct{})
	var result []defaultConstant
	for _, page := range pages {
		for _, param := range page.QueryParams {
			if param.DefaultLiteral == "" {
				continue
			}
			name := defaultConstName(page.Name, param.Name, param.DefaultLiteral)
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			result = append(result, defaultConstant{name: name, value: param.DefaultLiteral})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

func formGroupName(forms []pageInfo) string {
	if len(forms) == 0 {
		return ""
	}
	path := strings.Trim(forms[0].Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	segment := parts[0]
	segment = strings.Trim(segment, "{}")
	return segment
}
