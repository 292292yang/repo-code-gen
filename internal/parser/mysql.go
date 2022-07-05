package parser

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"repo-code-gen/internal/model"
	"repo-code-gen/internal/strcase"
)

var createTableRE = regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?((?:` + "`" + `[^` + "`" + `]+` + "`" + `|[a-zA-Z0-9_]+)(?:\.(?:` + "`" + `[^` + "`" + `]+` + "`" + `|[a-zA-Z0-9_]+))?)\s*\(`)

// ParseMySQLDDL parses one or more MySQL CREATE TABLE statements.
func ParseMySQLDDL(sqlText string) ([]model.Table, error) {
	cleaned := stripComments(sqlText)
	matches := createTableRE.FindAllStringSubmatchIndex(cleaned, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no CREATE TABLE statements found")
	}

	var tables []model.Table
	for _, m := range matches {
		tableNameRaw := cleaned[m[2]:m[3]]
		openParen := m[1] - 1
		closeParen, err := findMatchingParen(cleaned, openParen)
		if err != nil {
			return nil, err
		}
		body := cleaned[openParen+1 : closeParen]
		table, err := parseTable(cleanTableName(tableNameRaw), body)
		if err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func parseTable(name, body string) (model.Table, error) {
	t := model.Table{Name: name, GoName: strcase.ToCamel(name)}
	definitions := splitTopLevel(body, ',')
	primaryCols := []string{}

	for _, def := range definitions {
		def = strings.TrimSpace(def)
		if def == "" {
			continue
		}
		upper := strings.ToUpper(def)

		switch {
		case strings.HasPrefix(upper, "PRIMARY KEY"):
			cols := parseIndexColumns(def)
			primaryCols = cols
			t.PrimaryKey = &model.Index{Name: "PRIMARY", Columns: cols, GoName: joinColumnGoNames(cols), IsPrimary: true, IsUnique: true}
		case strings.HasPrefix(upper, "UNIQUE KEY") || strings.HasPrefix(upper, "UNIQUE INDEX") || strings.HasPrefix(upper, "UNIQUE "):
			idx := parseIndex(def, true)
			if len(idx.Columns) > 0 {
				t.UniqueKeys = append(t.UniqueKeys, idx)
			}
		case strings.HasPrefix(upper, "KEY ") || strings.HasPrefix(upper, "INDEX ") || strings.HasPrefix(upper, "FULLTEXT KEY") || strings.HasPrefix(upper, "FULLTEXT INDEX"):
			idx := parseIndex(def, false)
			if len(idx.Columns) > 0 {
				t.NormalKeys = append(t.NormalKeys, idx)
			}
		case strings.HasPrefix(upper, "CONSTRAINT "):
			if strings.Contains(upper, " PRIMARY KEY") {
				cols := parseIndexColumns(def)
				primaryCols = cols
				t.PrimaryKey = &model.Index{Name: "PRIMARY", Columns: cols, GoName: joinColumnGoNames(cols), IsPrimary: true, IsUnique: true}
			} else if strings.Contains(upper, " UNIQUE ") {
				idx := parseIndex(def, true)
				if len(idx.Columns) > 0 {
					t.UniqueKeys = append(t.UniqueKeys, idx)
				}
			}
		default:
			col, ok := parseColumn(def)
			if ok {
				t.Columns = append(t.Columns, col)
				if col.IsPrimaryKey {
					primaryCols = append(primaryCols, col.Name)
				}
			}
		}
	}

	if len(primaryCols) > 0 {
		if t.PrimaryKey == nil {
			t.PrimaryKey = &model.Index{Name: "PRIMARY", Columns: primaryCols, GoName: joinColumnGoNames(primaryCols), IsPrimary: true, IsUnique: true}
		}
		for i := range t.Columns {
			if containsString(primaryCols, t.Columns[i].Name) {
				t.Columns[i].IsPrimaryKey = true
			}
		}
	}

	return t, nil
}

func parseColumn(def string) (model.Column, bool) {
	name, rest, ok := readLeadingIdent(def)
	if !ok {
		return model.Column{}, false
	}
	name = cleanIdentifier(name)
	if name == "" {
		return model.Column{}, false
	}

	typeText := readType(rest)
	base, unsigned := normalizeDBType(typeText)
	upperRest := strings.ToUpper(rest)
	nullable := true
	if strings.Contains(upperRest, "NOT NULL") {
		nullable = false
	}
	if strings.Contains(upperRest, "PRIMARY KEY") {
		nullable = false
	}

	goType, needsTime, needsSQLNull := goTypeFor(base, typeText, nullable)
	goName := strcase.ToCamel(name)

	col := model.Column{
		Name:         name,
		GoName:       goName,
		VarName:      strcase.SafeIdent(strcase.ToLowerCamel(name)),
		DBType:       strings.TrimSpace(typeText),
		BaseDBType:   base,
		GoType:       goType,
		Nullable:     nullable,
		Unsigned:     unsigned,
		IsPrimaryKey: strings.Contains(upperRest, "PRIMARY KEY"),
		IsAutoIncr:   strings.Contains(upperRest, "AUTO_INCREMENT"),
		HasDefault:   strings.Contains(upperRest, "DEFAULT"),
		IsGenerated:  strings.Contains(upperRest, "GENERATED ALWAYS") || strings.Contains(upperRest, " AS (") || strings.Contains(upperRest, " AS("),
		Comment:      parseComment(rest),
		JSONTag:      name,
		NeedsTime:    needsTime,
		NeedsSQLNull: needsSQLNull,
	}
	return col, true
}

func readLeadingIdent(s string) (ident, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", false
	}
	if s[0] == '`' || s[0] == '"' {
		quote := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == quote {
				return s[:i+1], strings.TrimSpace(s[i+1:]), true
			}
		}
		return "", "", false
	}
	for i, r := range s {
		if unicode.IsSpace(r) {
			return s[:i], strings.TrimSpace(s[i:]), true
		}
	}
	return s, "", true
}

func readType(rest string) string {
	tokens := splitBySpaceTopLevel(rest)
	if len(tokens) == 0 {
		return ""
	}
	keywords := map[string]bool{
		"NOT": true, "NULL": true, "DEFAULT": true, "COMMENT": true, "COLLATE": true,
		"CHARACTER": true, "AUTO_INCREMENT": true, "PRIMARY": true, "UNIQUE": true, "KEY": true,
		"GENERATED": true, "AS": true, "VIRTUAL": true, "STORED": true, "REFERENCES": true,
		"CHECK": true, "ON": true, "UPDATE": true,
	}
	var out []string
	for _, tok := range tokens {
		upper := strings.ToUpper(strings.TrimSpace(tok))
		if keywords[upper] {
			break
		}
		out = append(out, tok)
		if upper == "UNSIGNED" || upper == "ZEROFILL" {
			continue
		}
	}
	return strings.Join(out, " ")
}

func normalizeDBType(typeText string) (base string, unsigned bool) {
	lower := strings.ToLower(strings.TrimSpace(typeText))
	unsigned = strings.Contains(lower, "unsigned")
	if idx := strings.Index(lower, "("); idx >= 0 {
		lower = lower[:idx]
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return "string", unsigned
	}
	base = fields[0]
	return base, unsigned
}

func goTypeFor(base, typeText string, nullable bool) (goType string, needsTime bool, needsSQLNull bool) {
	lowerType := strings.ToLower(typeText)
	unsigned := strings.Contains(lowerType, "unsigned")
	baseType := "string"
	switch base {
	case "tinyint":
		if strings.HasPrefix(lowerType, "tinyint(1)") || strings.HasPrefix(lowerType, "tinyint (1)") {
			baseType = "bool"
		} else if unsigned {
			baseType = "uint64"
		} else {
			baseType = "int64"
		}
	case "smallint", "mediumint", "int", "integer", "year":
		if unsigned {
			baseType = "uint64"
		} else {
			baseType = "int64"
		}
	case "bigint":
		if unsigned {
			baseType = "uint64"
		} else {
			baseType = "int64"
		}
	case "float", "double", "decimal", "numeric", "real":
		baseType = "float64"
	case "bool", "boolean":
		baseType = "bool"
	case "date", "datetime", "timestamp", "time":
		baseType = "time.Time"
		needsTime = true
	case "binary", "varbinary", "blob", "tinyblob", "mediumblob", "longblob":
		baseType = "[]byte"
	default:
		baseType = "string"
	}
	if nullable && baseType != "[]byte" {
		return "*" + baseType, needsTime, false
	}
	return baseType, needsTime, false
}

func parseIndex(def string, unique bool) model.Index {
	cols := parseIndexColumns(def)
	idx := model.Index{Columns: cols, GoName: joinColumnGoNames(cols), IsUnique: unique}
	parts := splitBySpaceTopLevel(def)
	for i, p := range parts {
		up := strings.ToUpper(p)
		if up == "KEY" || up == "INDEX" {
			if i+1 < len(parts) && !strings.HasPrefix(parts[i+1], "(") {
				idx.Name = cleanIdentifier(parts[i+1])
			}
			break
		}
	}
	if idx.Name == "" && len(cols) > 0 {
		idx.Name = strings.Join(cols, "_")
	}
	return idx
}

func parseIndexColumns(def string) []string {
	start := strings.Index(def, "(")
	if start < 0 {
		return nil
	}
	end, err := findMatchingParen(def, start)
	if err != nil || end <= start {
		return nil
	}
	inside := def[start+1 : end]
	parts := splitTopLevel(inside, ',')
	cols := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ident, _, ok := readLeadingIdent(p)
		if ok {
			cols = append(cols, cleanIdentifier(ident))
		}
	}
	return cols
}

func parseComment(rest string) string {
	upper := strings.ToUpper(rest)
	idx := strings.Index(upper, "COMMENT")
	if idx < 0 {
		return ""
	}
	after := strings.TrimSpace(rest[idx+len("COMMENT"):])
	if after == "" || after[0] != '\'' {
		return ""
	}
	var b strings.Builder
	escaped := false
	for i := 1; i < len(after); i++ {
		c := after[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == '\'' {
			break
		}
		b.WriteByte(c)
	}
	return b.String()
}

func joinColumnGoNames(cols []string) string {
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		parts = append(parts, strcase.ToCamel(c))
	}
	return strings.Join(parts, "And")
}

func cleanTableName(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ".") {
		parts := strings.Split(s, ".")
		s = parts[len(parts)-1]
	}
	return cleanIdentifier(s)
}

func cleanIdentifier(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`\" ")
	return s
}

func containsString(values []string, needle string) bool {
	for _, v := range values {
		if v == needle {
			return true
		}
	}
	return false
}

func splitBySpaceTopLevel(s string) []string {
	var out []string
	var b strings.Builder
	depth := 0
	quote := rune(0)
	escaped := false
	flush := func() {
		if strings.TrimSpace(b.String()) != "" {
			out = append(out, strings.TrimSpace(b.String()))
		}
		b.Reset()
	}
	for _, r := range s {
		if quote != 0 {
			b.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			quote = r
			b.WriteRune(r)
		case '(':
			depth++
			b.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
		default:
			if unicode.IsSpace(r) && depth == 0 {
				flush()
			} else {
				b.WriteRune(r)
			}
		}
	}
	flush()
	return out
}

func splitTopLevel(s string, sep rune) []string {
	var out []string
	var b strings.Builder
	depth := 0
	quote := rune(0)
	escaped := false
	for _, r := range s {
		if quote != 0 {
			b.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"', '`':
			quote = r
			b.WriteRune(r)
		case '(':
			depth++
			b.WriteRune(r)
		case ')':
			if depth > 0 {
				depth--
			}
			b.WriteRune(r)
		default:
			if r == sep && depth == 0 {
				out = append(out, strings.TrimSpace(b.String()))
				b.Reset()
			} else {
				b.WriteRune(r)
			}
		}
	}
	if strings.TrimSpace(b.String()) != "" {
		out = append(out, strings.TrimSpace(b.String()))
	}
	return out
}

func findMatchingParen(s string, open int) (int, error) {
	if open < 0 || open >= len(s) || s[open] != '(' {
		return -1, fmt.Errorf("invalid opening parenthesis position")
	}
	depth := 0
	quote := byte(0)
	escaped := false
	for i := open; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return -1, fmt.Errorf("unmatched opening parenthesis")
}

func stripComments(s string) string {
	var b strings.Builder
	quote := byte(0)
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			b.WriteByte(c)
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' || c == '`' {
			quote = c
			b.WriteByte(c)
			continue
		}
		if c == '-' && i+1 < len(s) && s[i+1] == '-' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
			continue
		}
		if c == '#' {
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
			continue
		}
		if c == '/' && i+1 < len(s) && s[i+1] == '*' {
			i += 2
			for i+1 < len(s) && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i++
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
