# Botx YAML to Code Generation Guide

This guide explains every field in `pkg/codegen/types.go`, how each field is intended to work, and how the generator should emit Go code. It also explains the hybrid generation pattern used by `StringExpr` and `Code`.

The goal: an intern should be able to implement a working generator from this document.

## 1. Core Concepts

### 1.1 Hybrid generation with `StringExpr` and `Code`

`StringExpr` and `Code` are string aliases that carry code fragments instead of runtime data. They are embedded into the generated Go source directly. This is the hybrid generation pattern: static scaffolding with user‑authored snippets.

- **`StringExpr`**: Template-friendly string expression. Used for message text, labels, tips, and other user-visible text. If the value includes `${...}`, the generator copies the expression content into Go code so it is evaluated at compile time.
- **`Code`**: Pure code fragments. Used where the YAML needs to refer to Go expressions (pagination sizing, totals, etc.).

**Why this pattern?**
- Compile-time verification: invalid expressions fail to compile instead of failing at runtime.
- No runtime template engine required.
- The generated code stays idiomatic and debuggable.

**Example**

```yaml
view:
  message: |
    ${cond(len(state) == 0, "empty", "")}
```

The generator emits the expression directly into Go’s `fmt.Sprintf` in the view renderer.

## 2. Full Type Reference

This section mirrors `pkg/codegen/types.go` and explains every field in plain language, plus how the generator should use it.

### 2.1 Doc

```go
type Doc struct {
    Navbar     *Navbar         `yaml:"navbar,omitempty"`
    Pages      map[string]Page `yaml:"pages"`
    API        map[string]API  `yaml:"api"`
    Components Components      `yaml:"components,omitempty"`
}
```

- `navbar`: Optional shared navigation bar rendered on pages.
- `pages`: Mapping from route path to `Page` definition. Keys are paths like `/address/{ID}`.
- `api`: API descriptors used for generating typed helper functions.
- `components`: Shared schemas used by `Page.state` or `Form` items.

### 2.2 Navbar

Navbar is an alias of `ButtonGrid`.

```go
type Navbar ButtonGrid
```

Any `ButtonGrid` example can be used as `navbar`.

**Example**

```yaml
navbar:
  rows:
    - columns:
      - label: 返回
        onClick: back
      - label: 返回主页
        onClick: /
```

### 2.3 ButtonGrid, ButtonGridRow, Button

```go
type ButtonGrid struct {
    Rows []ButtonGridRow `yaml:"rows"`
}

type ButtonGridRow struct {
    Columns []Button `yaml:"columns"`
}

type Button struct {
    Label   StringExpr `yaml:"label"`
    OnClick StringExpr `yaml:"onClick"`
}
```

**Semantics**
- `ButtonGrid` is a 2D layout.
- Each `ButtonGridRow` is a row of buttons.
- `Button.Label` is the display text (supports `StringExpr`).
- `Button.OnClick` determines callback data or navigation (supports `StringExpr`).

**Example**

```yaml
grid:
  rows:
    - columns:
      - label: 返回
        onClick: back
      - label: 返回主页
        onClick: /
```

**Generation**
- The generator produces a `[][]bot.Button` with row/column structure.
- `OnClick` values generate `bot.Route(...)` or special route `back`.

### 2.4 Form

```go
type Form struct {
    Required []string             `yaml:"required,omitempty"`
    Fields   map[string]FormField `yaml:"fields,omitempty"`
}
```

**Semantics**
- `required`: Fields that must be present in submission.
- `fields`: Map of field name to `FormField`. Field names are preserved as written in YAML.

**Generation**
- Generator emits a `Form*` struct with private fields and `Get*()` accessors.
- Required fields are validated in the `unmarshalForm*` function.

### 2.5 FormField

```go
type FormField struct {
    Label     StringExpr      `yaml:"label,omitempty"`
    Input     *FormFieldInput `yaml:"input,omitempty"`
    Validator *StringExpr     `yaml:"validator,omitempty"`
}
```

**Semantics**
- `label`: User-visible label.
- `input`: Input type and hint text.
- `validator`: Name of a validation function. The generator will emit a `FormValidator` interface method for each unique validator name.

**Generation**
- For each field, generator creates a `bot.FormField` with `Label`, `Input`, and `Validator`.
- If `validator` is set, the generated `FormValidator` interface gets a method like `ValidateFormAddressAdd(...)` or `Validate<ValidatorName>(...)` depending on generator conventions.

### 2.6 FormFieldInput

```go
type FormFieldInput struct {
    Type string     `yaml:"type,omitempty"`
    Tip  StringExpr `yaml:"tip,omitempty"`
}
```

**Semantics**
- `type`: Input type (text, number, etc.).
- `tip`: Instruction text; supports `StringExpr`.

**Generation**
- `Type` maps to input schema type in the form message.
- `Tip` becomes the form hint text.

### 2.7 Page

```go
type Page struct {
    Parameters map[string]*openapi3.Parameter `yaml:"parameters,omitempty"`
    State      *openapi3.Schema               `yaml:"state,omitempty"`
    Form       *Form                          `yaml:"form,omitempty"`
    View       View                           `yaml:"view,omitempty"`
    Redirect   *StringExpr                    `yaml:"redirect,omitempty"`
}
```

**Semantics**
- `parameters`: Query/path parameters, preserved by name (e.g. `ID`, `page`).
- `state`: Schema for view data.
- `form`: Optional form for user input.
- `view`: Presentation of the page.
- `redirect`: Optional redirect expression.

**Generation**
- Parameters become parser functions and parameter types with `Get*()` accessors.
- `state` becomes a state struct with private fields and `Get*()` accessors.
- `form` generates a form struct, form renderer, and unmarshal logic.
- `view` generates rendering functions for messages and buttons.

### 2.9 View

```go
type View struct {
    ParseMode *models.ParseMode `yaml:"parseMode,omitempty"`
    Message   *StringExpr       `yaml:"message,omitempty"`
    Buttons   *Buttons          `yaml:"buttons,omitempty"`
}
```

**Semantics**
- `parseMode`: Telegram parse mode (HTML/Markdown).
- `message`: The message template (`StringExpr`). This is inserted into generated Go source. Any `${...}` expression is written directly into the code, so invalid expressions fail at compile time.
- `buttons`: Button layout.

**Generation**
- `message` is interpolated into Go `fmt.Sprintf`, with expressions emitted directly.
- `buttons` are rendered into a `[][]bot.Button`.

### 2.10 Buttons

```go
type Buttons struct {
    Grids      []ButtonGrider `yaml:"grids,omitempty"`
    Grid       ButtonGrider   `yaml:"grid,omitempty"`
    Pagination ButtonGrider   `yaml:"pagination,omitempty"`
}
```

**Semantics**
- Only one of `grids`, `grid`, or `pagination` should be set.
- `grids` allows merging multiple grids in order.
- `ButtonGrider` is an internal marker type, allowing either `ButtonGrid` or `Pagination` to be assigned.

**Generation**
- `grid`: Emit a single button grid.
- `grids`: Emit each grid in order and append into a final grid.
- `pagination`: Emit a pagination grid with item templates.

### 2.11 Pagination

```go
type Pagination struct {
    Row    Code   `yaml:"row,omitempty"`
    Column Code   `yaml:"column,omitempty"`
    Page   Code   `yaml:"page,omitempty"`
    State  Code   `yaml:"state,omitempty"`
    Item   Button `yaml:"item,omitempty"`
}
```

**Semantics**
- `row`, `column`: Per-page layout expressions.
- `page`: Current page index expression.
- `state`: Total count expression (used to compute last page).
- `item`: Button template for each item in the page slice.

**Generation**
- Emit a `pagination` helper call with these expressions.
- The `item` button is used to render each data item.

### 2.12 API and Arg

```go
type API struct {
    Args []*Arg `yaml:"args,omitempty"`
}

type Arg struct {
    Name   string              `yaml:"name"`
    Schema *openapi3.SchemaRef `yaml:"schema,omitempty"`
}
```

**Semantics**
- `api` describes helper functions or endpoints.
- `args` describes typed input parameters.

**Generation**
- Generator emits strongly typed function signatures based on `args`.

### 2.13 Components

```go
type Components struct {
    Schemas map[string]*openapi3.Schema `yaml:"schemas,omitempty"`
}
```

**Semantics**
- Reusable schemas for page state, form data, and lists.

**Generation**
- Emit Go structs with fields preserved from YAML (e.g. `ID`).
- Private fields use getters (`Get*()`) in the generated code.

## 3. Validator Behavior

When a form field has a validation rule, generator must emit a validation hook.

Example:

```yaml
form:
  fields:
    address:
      validator: validateAddressOrName
```

**Generation**
- Produce a `FormValidator` interface with methods for each validator name.
- The generator calls the validator method and expects a `ValidateResult` response.
- This makes validation part of the generated API contract and keeps failures compile-time visible.

## 4. Generator Implementation Steps

1. Parse YAML into `Doc`.
2. Validate `Buttons` rules (only one of grids/grid/pagination).
3. For each `Page`:
   - Emit parameter parser.
   - Emit state struct and getters.
   - Emit form struct and unmarshal logic if `form` exists.
   - Emit view renderer based on `view.message` and `view.buttons`.
4. Emit `FormValidator` and `StateProvider` interfaces.
5. Emit routing dispatch and form submit dispatch.
6. Emit helpers (`pagination`, `cond`, `forEach`).

## 5. Generated File Structure (botx_gen.go)

The generator writes a single Go file like `samples/common/botx_gen.go`. The file is organized into sections separated by comments. Each section maps back to parts of the YAML.

### 5.1 Schemas
Source: `components.schemas` in YAML.

```go
// schemas

type Address struct {
	ID      int64
	address string
	name    string
}
```

Schema field names preserve the YAML casing (`ID` stays `ID`). Private fields get `Get*()` accessors.

### 5.2 Core handler
Source: framework boilerplate + YAML pages/validators.

```go
type BotxHandler struct {
	renderer       *PageRenderer
	sm             session.SessionManager
	sp             StateProvider
	formValidator  FormValidator
	commandHandler CommandHandler
	defaultHandler Handler
}
```

This section wires routing, form submission, error handling, validation dispatch, and command handlers defined in YAML `handlers`.

For example, `/start` generates a `CommandHandler` interface method and is dispatched in `HandleTextMessage`.

### 5.3 Route matchers and dispatch
Source: `pages` keys, including path parameters.

```go
// code generated for pages

var (
	addressIDMatcher     = routepath.MustCompile("/address/{ID}")
	addressDeleteMatcher = routepath.MustCompile("/address/{ID}/delete")
	addressEditMatcher   = routepath.MustCompile("/address/{ID}/edit")
)
```

The generator creates matchers for any route containing `{param}`. These are used in `onRoute` and `onSubmit` to pick the correct page.

### 5.4 Route handling
Source: each `pages` entry.

```go
func (h *BotxHandler) onRoute(...) error {
	switch {
	case url.Path == "/address":
		params, _ := ParseParametersPageAddress(url)
		state, _ := h.sp.ProvideAddressState(...)
		return h.renderer.pageAddress(...)
	case okAddressID:
		params, _ := ParseParametersPageAddressID(paramsAddressID)
		state, _ := h.sp.ProvideAddressIDState(...)
		return h.renderer.pageAddressID(...)
	}
}
```

Each page generates a case that parses parameters, calls a `StateProvider` method, and renders a view.

### 5.5 Submit handling
Source: `form` sections under `pages`.

```go
func (h *BotxHandler) onSubmit(...) error {
	switch {
	case url.Path == "/address/add":
		form, _ := unmarshalFormAddressAdd(values)
		state, _ := h.sp.ProvideAddressAddState(...)
		return h.renderer.pageAddressAdd(...)
	}
}
```

Each form generates an `unmarshalForm*` function plus a submit branch that calls `StateProvider` and renders a result page.

### 5.6 Parameter parsing
Source: `page.parameters`.

```go
func ParseParametersPageAddress(url *url.URL) (*ParametersPageAddress, error) {
	column, _ := parseOptionalIntQuery(url, "column", defaultAddressColumns)
	return &ParametersPageAddress{column: column, row: row, page: page}, nil
}
```

Query/path parameters generate parser functions plus parameter structs with `Get*()` accessors.

### 5.7 Forms and validators
Source: `form.fields` and `form.fields.*.validator`.

```go
type FormAddressAdd struct {
	address string
}

type FormValidator interface {
	ValidateFormAddressAdd(ctx context.Context, chatID int64, url *url.URL, input string) (*bot.ValidateResult, error)
}
```

Each field becomes a form struct field and getter. Each validator name generates a method in `FormValidator` that the user must implement.

### 5.8 State provider
Source: `pages` and their `state` definitions.

```go
type StateProvider interface {
	ProvideAddressState(ctx context.Context, chatID int64, parameters *ParametersPageAddress) (*StatePageAddress, error)
}
```

Every page generates a `Provide*State` method so users supply data for rendering.

### 5.9 Page renderer
Source: `view.message`, `view.buttons`, `form`.

```go
func (p *PageRenderer) pageAddress(...) error {
	return p.b.SendMessage(ctx, chatID, &bot.Message{...})
}
```

Message expressions are embedded via `StringExpr`. Button grids reflect `ButtonGrid` and `Buttons` settings.

### 5.10 Helpers and utils
Source: generator runtime helpers.

```go
func pagination[T any](columns int, rows int, total int, page int, items []T, castFunc func(item T) bot.Button, prevLabel string, nextLabel string) [][]bot.Button
```

Helpers like `pagination`, `cond`, and `forEach` support common template expressions.

## 6. Example Generated Code

```go
type Address struct {
	ID      int64
	address string
	name    string
}

func (a Address) GetAddress() string { return a.address }
func (a Address) GetName() string    { return a.name }

type FormAddressAdd struct {
	address string
}

func (f *FormAddressAdd) GetAddress() string { return f.address }
```
### 5.2.1 Command handlers
Source: `handlers` section in YAML.

```go
type CommandHandler interface {
	HandleCommandStart(ctx context.Context, chatID int64, b bot.Bot, router *bot.Router) error
}
```

The generator creates an interface method per handler entry and wires it into `HandleTextMessage` by matching the incoming text.
