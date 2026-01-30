package codegen

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

func (p *Parameters) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Kind == 0 {
		return nil
	}

	switch node.Kind {
	case yaml.MappingNode:
		var raw map[string]*openapi3.Parameter
		if err := node.Decode(&raw); err != nil {
			return err
		}
		for name, param := range raw {
			if param == nil {
				continue
			}
			if param.Name == "" {
				param.Name = name
			}
		}
		*p = raw
		return nil
	case yaml.SequenceNode:
		var list []*openapi3.Parameter
		if err := node.Decode(&list); err != nil {
			return err
		}
		result := make(map[string]*openapi3.Parameter, len(list))
		for _, param := range list {
			if param == nil {
				continue
			}
			if param.Name == "" {
				return fmt.Errorf("parameter name is required")
			}
			result[param.Name] = param
		}
		*p = result
		return nil
	default:
		return fmt.Errorf("parameters must be a map or list")
	}
}

func (f *FormFieldInput) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Type   string           `yaml:"type"`
		Format string           `yaml:"format"`
		Tip    StringExpr       `yaml:"tip"`
		Schema *openapi3.Schema `yaml:"schema"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if raw.Type == "" && raw.Schema != nil {
		if raw.Schema.Type != nil && len(*raw.Schema.Type) != 0 {
			raw.Type = (*raw.Schema.Type)[0]
		}
		if raw.Format == "" {
			raw.Format = raw.Schema.Format
		}
	}
	f.Type = raw.Type
	f.Format = raw.Format
	f.Tip = raw.Tip
	return nil
}

func (p *Pagination) UnmarshalYAML(node *yaml.Node) error {
	var raw struct {
		Rows      Code       `yaml:"rows"`
		Row       Code       `yaml:"row"`
		Columns   Code       `yaml:"columns"`
		Column    Code       `yaml:"column"`
		Page      Code       `yaml:"page"`
		Total     Code       `yaml:"total"`
		State     Code       `yaml:"state"`
		Items     Code       `yaml:"items"`
		Item      Button     `yaml:"item"`
		PrevLabel StringExpr `yaml:"prevLabel"`
		NextLabel StringExpr `yaml:"nextLabel"`
	}
	if err := node.Decode(&raw); err != nil {
		return err
	}
	if raw.Rows == "" {
		raw.Rows = raw.Row
	}
	if raw.Columns == "" {
		raw.Columns = raw.Column
	}
	if raw.Total == "" {
		raw.Total = raw.State
	}
	p.Rows = raw.Rows
	p.Columns = raw.Columns
	p.Page = raw.Page
	p.Total = raw.Total
	p.Items = raw.Items
	p.Item = raw.Item
	p.PrevLabel = raw.PrevLabel
	p.NextLabel = raw.NextLabel
	return nil
}

func (b *Buttons) UnmarshalYAML(node *yaml.Node) error {
	gridsNode := mappingValue(node, "grids")
	gridNode := mappingValue(node, "grid")
	paginationNode := mappingValue(node, "pagination")
	if gridNode != nil {
		grid, err := decodeButtonGridNode(gridNode)
		if err != nil {
			return err
		}
		b.Grid = grid
	}
	if paginationNode != nil {
		var pagination Pagination
		if err := paginationNode.Decode(&pagination); err != nil {
			return err
		}
		b.Pagination = pagination
	}
	if gridsNode != nil && gridsNode.Kind == yaml.SequenceNode {
		b.Grids = make([]ButtonGrider, 0, len(gridsNode.Content))
		for _, item := range gridsNode.Content {
			if item == nil {
				continue
			}
			grid, err := decodeButtonGridItem(item)
			if err != nil {
				return err
			}
			b.Grids = append(b.Grids, grid)
		}
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func decodeButtonGridItem(node *yaml.Node) (ButtonGrider, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("button grid item must be a map")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]
		switch strings.ToLower(key.Value) {
		case "pagination":
			var pagination Pagination
			if err := value.Decode(&pagination); err != nil {
				return nil, err
			}
			return pagination, nil
		case "grid":
			return decodeButtonGridNode(value)
		case "rows":
			return decodeButtonGridNode(node)
		}
	}
	return decodeButtonGridNode(node)
}

func decodeButtonGridNode(node *yaml.Node) (ButtonGrid, error) {
	var grid ButtonGrid
	if err := node.Decode(&grid); err != nil {
		return ButtonGrid{}, err
	}
	return grid, nil
}
