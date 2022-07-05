package strcase

import (
	"regexp"
	"strings"
	"unicode"
)

var commonInitialisms = map[string]string{
	"id": "ID", "url": "URL", "uri": "URI", "api": "API", "ip": "IP", "uuid": "UUID",
	"json": "JSON", "xml": "XML", "html": "HTML", "http": "HTTP", "https": "HTTPS",
	"sql": "SQL", "db": "DB", "uid": "UID", "ttl": "TTL", "tcp": "TCP", "udp": "UDP",
}

var nonWord = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// ToCamel converts snake_case, kebab-case, or space separated text to ExportedCamelCase.
func ToCamel(s string) string {
	parts := splitParts(s)
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		lower := strings.ToLower(p)
		if v, ok := commonInitialisms[lower]; ok {
			b.WriteString(v)
			continue
		}
		runes := []rune(lower)
		if len(runes) == 0 {
			continue
		}
		b.WriteRune(unicode.ToUpper(runes[0]))
		for _, r := range runes[1:] {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "X"
	}
	if unicode.IsDigit([]rune(out)[0]) {
		return "X" + out
	}
	return out
}

// ToLowerCamel converts text to lowerCamelCase while preserving common initialisms sensibly.
func ToLowerCamel(s string) string {
	camel := ToCamel(s)
	if camel == "" {
		return "x"
	}
	for _, init := range commonInitialisms {
		if strings.HasPrefix(camel, init) && len(camel) > len(init) {
			return strings.ToLower(init) + camel[len(init):]
		}
		if camel == init {
			return strings.ToLower(init)
		}
	}
	runes := []rune(camel)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func splitParts(s string) []string {
	s = strings.TrimSpace(strings.Trim(s, "`\"'"))
	if s == "" {
		return nil
	}
	s = nonWord.ReplaceAllString(s, "_")
	return strings.FieldsFunc(s, func(r rune) bool { return r == '_' })
}

// SafeIdent returns a Go identifier that avoids common reserved words.
func SafeIdent(s string) string {
	if s == "" {
		return "v"
	}
	reserved := map[string]bool{
		"break": true, "default": true, "func": true, "interface": true, "select": true,
		"case": true, "defer": true, "go": true, "map": true, "struct": true,
		"chan": true, "else": true, "goto": true, "package": true, "switch": true,
		"const": true, "fallthrough": true, "if": true, "range": true, "type": true,
		"continue": true, "for": true, "import": true, "return": true, "var": true,
	}
	if reserved[s] {
		return s + "Value"
	}
	return s
}
