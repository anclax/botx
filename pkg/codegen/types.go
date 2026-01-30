package codegen

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-telegram/bot/models"
)

type (
	Code       string
	StringExpr string
)

type ButtonGrider any

// ButtonGrid represents a grid of buttons.
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

type Form struct {
	Required []string             `yaml:"required,omitempty"`
	Fields   map[string]FormField `yaml:"fields,omitempty"`
}

type FormField struct {
	Label     StringExpr      `yaml:"label,omitempty"`
	Input     *FormFieldInput `yaml:"input,omitempty"`
	Validator *StringExpr     `yaml:"validator,omitempty"`
}

type FormFieldInput struct {
	Type   string     `yaml:"type,omitempty"`
	Format string     `yaml:"format,omitempty"`
	Tip    StringExpr `yaml:"tip,omitempty"`
}

type Navbar ButtonGrid

type Doc struct {
	Package    string          `yaml:"package,omitempty"`
	I18n       *I18n           `yaml:"i18n,omitempty"`
	Navbar     *Navbar         `yaml:"navbar,omitempty"`
	Handlers   []Handler       `yaml:"handlers,omitempty"`
	Pages      map[string]Page `yaml:"pages"`
	API        map[string]API  `yaml:"api"`
	Components Components      `yaml:"components,omitempty"`
}

type Page struct {
	Parameters Parameters       `yaml:"parameters,omitempty"`
	State      *openapi3.Schema `yaml:"state,omitempty"`
	Form       *Form            `yaml:"form,omitempty"`
	View       View             `yaml:"view,omitempty"`
	Redirect   *StringExpr      `yaml:"redirect,omitempty"`
}

type Arg struct {
	Name   string              `yaml:"name"`
	Schema *openapi3.SchemaRef `yaml:"schema,omitempty"`
}

type API struct {
	Args []*Arg `yaml:"args,omitempty"`
}

type Handler struct {
	Match     StringExpr `yaml:"match"`
	MatchType string     `yaml:"matchType"`
	Type      string     `yaml:"type"`
	Action    StringExpr `yaml:"action"`
}

type View struct {
	ParseMode *models.ParseMode `yaml:"parseMode,omitempty"`
	Message   *StringExpr       `yaml:"message,omitempty"`
	Buttons   *Buttons          `yaml:"buttons,omitempty"`
}

type Buttons struct {
	// Grids is a list of button grids, All items will be merged into one button grid in order.
	// Only one of the Girds, Grid, or Pagination can be set.
	Grids []ButtonGrider `yaml:"grids,omitempty"`

	// Grid is a single button grid. Only one of the Girds, Grid, or Pagination can be set.s
	Grid ButtonGrider `yaml:"grid,omitempty"`

	// Pagination is a pagination button grid. Only one of the Girds, Grid, or Pagination can be set.
	Pagination ButtonGrider `yaml:"pagination,omitempty"`
}

type Pagination struct {
	Rows      Code       `yaml:"rows,omitempty"`
	Columns   Code       `yaml:"columns,omitempty"`
	Page      Code       `yaml:"page,omitempty"`
	Total     Code       `yaml:"total,omitempty"`
	Items     Code       `yaml:"items,omitempty"`
	Item      Button     `yaml:"item,omitempty"`
	PrevLabel StringExpr `yaml:"prevLabel,omitempty"`
	NextLabel StringExpr `yaml:"nextLabel,omitempty"`
}

type Components struct {
	Schemas map[string]*openapi3.Schema `yaml:"schemas,omitempty"`
}

type I18n struct {
	Default string                       `yaml:"default,omitempty"`
	Entries map[string]map[string]string `yaml:"entries,omitempty"`
}

type Parameters map[string]*openapi3.Parameter
