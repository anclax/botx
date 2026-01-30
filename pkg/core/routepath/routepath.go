package routepath

import (
	"fmt"
	"regexp"
	"strings"
)

type Params struct {
	values map[string]string
}

func (p Params) Get(key string) (string, bool) {
	if p.values == nil {
		return "", false
	}
	value, ok := p.values[key]
	return value, ok
}

type Matcher struct {
	re     *regexp.Regexp
	params []string
}

func MustCompile(pattern string) Matcher {
	matcher, err := Compile(pattern)
	if err != nil {
		panic(err)
	}
	return matcher
}

func Compile(pattern string) (Matcher, error) {
	if !strings.HasPrefix(pattern, "/") {
		return Matcher{}, fmt.Errorf("routepath: pattern must start with '/': %s", pattern)
	}
	var names []string
	var sb strings.Builder
	sb.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == '{' {
			end := strings.IndexByte(pattern[i:], '}')
			if end == -1 {
				return Matcher{}, fmt.Errorf("routepath: missing closing brace in %s", pattern)
			}
			name := pattern[i+1 : i+end]
			if name == "" {
				return Matcher{}, fmt.Errorf("routepath: empty param name in %s", pattern)
			}
			names = append(names, name)
			sb.WriteString("([^/]+)")
			i += end
			continue
		}
		sb.WriteString(regexp.QuoteMeta(string(ch)))
	}

	sb.WriteString("$")
	compiled, err := regexp.Compile(sb.String())
	if err != nil {
		return Matcher{}, fmt.Errorf("routepath: compile pattern %s: %w", pattern, err)
	}
	return Matcher{re: compiled, params: names}, nil
}

func (m Matcher) Match(path string) (Params, bool) {
	matches := m.re.FindStringSubmatch(path)
	if len(matches) == 0 {
		return Params{}, false
	}
	params := make(map[string]string, len(m.params))
	for i, name := range m.params {
		params[name] = matches[i+1]
	}
	return Params{values: params}, true
}
