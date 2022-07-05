package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"repo-code-gen/internal/model"
	"repo-code-gen/internal/strcase"
)

// Options controls repository code generation.
type Options struct {
	ModulePath            string
	DomainDir             string
	RepositoryDir         string
	ImplDir               string
	DomainPackage         string
	RepositoryPackage     string
	ImplPackage           string
	GenerateDomain        bool
	GenerateInterface     bool
	GenerateDelete        bool
	GenerateUniqueFinders bool
	Overwrite             bool
	DryRun                bool
}

// Result contains the paths written by the generator.
type Result struct {
	Written []string
	Skipped []string
}

// Generate writes files for all tables.
func Generate(tables []model.Table, opt Options) (Result, error) {
	if opt.DomainPackage == "" {
		opt.DomainPackage = "domain"
	}
	if opt.RepositoryPackage == "" {
		opt.RepositoryPackage = "repository"
	}
	if opt.ImplPackage == "" {
		opt.ImplPackage = "mysql"
	}
	if opt.DomainDir == "" {
		opt.DomainDir = "internal/domain"
	}
	if opt.RepositoryDir == "" {
		opt.RepositoryDir = "internal/repository"
	}
	if opt.ImplDir == "" {
		opt.ImplDir = "internal/repositoryimpl/mysql"
	}
	if opt.ModulePath == "" {
		return Result{}, fmt.Errorf("module path is required")
	}

	ctx := templateCtx{Options: opt}
	res := Result{}
	for _, tbl := range tables {
		if len(tbl.Columns) == 0 {
			return res, fmt.Errorf("table %s has no columns", tbl.Name)
		}
		if tbl.PrimaryKey == nil || len(tbl.PrimaryKey.Columns) != 1 {
			return res, fmt.Errorf("table %s must have exactly one primary key column", tbl.Name)
		}
		ctx.Table = enrichTable(tbl)
		if opt.GenerateDomain {
			path := filepath.Join(opt.DomainDir, snakeFileName(tbl.Name)+"_gen.go")
			if err := writeTemplate(path, domainTemplate, ctx, opt, &res); err != nil {
				return res, err
			}
		}
		if opt.GenerateInterface {
			path := filepath.Join(opt.RepositoryDir, snakeFileName(tbl.Name)+"_repository_gen.go")
			if err := writeTemplate(path, repositoryTemplate, ctx, opt, &res); err != nil {
				return res, err
			}
		}
		path := filepath.Join(opt.ImplDir, snakeFileName(tbl.Name)+"_repository_gen.go")
		if err := writeTemplate(path, mysqlImplTemplate, ctx, opt, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

type templateCtx struct {
	Options Options
	Table   viewTable
}

type viewTable struct {
	model.Table
	VarName            string
	ConstructorName    string
	PK                 viewColumn
	InsertColumns      []viewColumn
	UpdateColumns      []viewColumn
	AllColumns         []viewColumn
	UniqueFinders      []viewFinder
	ColumnStringValues []string
	NeedsTime          bool
	NeedsSQLNull       bool
}

type viewColumn struct {
	model.Column
	QuotedName string
}

type viewFinder struct {
	Name       string
	Columns    []viewColumn
	Params     string
	Args       string
	EqMap      string
	Private    string
	PrivateTx  string
	PrivateArg string
}

func enrichTable(t model.Table) viewTable {
	vt := viewTable{Table: t, VarName: strcase.ToLowerCamel(t.GoName), ConstructorName: "New" + t.GoName + "Repository"}
	for _, c := range t.Columns {
		vc := viewColumn{Column: c, QuotedName: fmt.Sprintf("%q", c.Name)}
		vt.AllColumns = append(vt.AllColumns, vc)
		vt.ColumnStringValues = append(vt.ColumnStringValues, fmt.Sprintf("%q", c.Name))
		if c.NeedsTime {
			vt.NeedsTime = true
		}
		if c.NeedsSQLNull {
			vt.NeedsSQLNull = true
		}
		if shouldInsert(c) {
			vt.InsertColumns = append(vt.InsertColumns, vc)
		}
		if shouldUpdate(c) {
			vt.UpdateColumns = append(vt.UpdateColumns, vc)
		}
		if t.PrimaryKey != nil && len(t.PrimaryKey.Columns) == 1 && c.Name == t.PrimaryKey.Columns[0] {
			vt.PK = vc
		}
	}
	seenFinder := map[string]bool{}
	for _, idx := range t.UniqueKeys {
		if len(idx.Columns) == 0 {
			continue
		}
		if t.PrimaryKey != nil && sameColumns(idx.Columns, t.PrimaryKey.Columns) {
			continue
		}
		finderCols := make([]viewColumn, 0, len(idx.Columns))
		for _, name := range idx.Columns {
			if col, ok := findViewColumn(vt.AllColumns, name); ok {
				finderCols = append(finderCols, col)
			}
		}
		if len(finderCols) != len(idx.Columns) {
			continue
		}
		name := "FindBy" + joinGoNames(finderCols)
		if seenFinder[name] {
			continue
		}
		seenFinder[name] = true
		vt.UniqueFinders = append(vt.UniqueFinders, makeFinder(name, finderCols))
	}
	return vt
}

func makeFinder(name string, cols []viewColumn) viewFinder {
	params := make([]string, 0, len(cols))
	args := make([]string, 0, len(cols))
	eq := make([]string, 0, len(cols))
	for _, c := range cols {
		params = append(params, fmt.Sprintf("%s %s", c.VarName, c.GoType))
		args = append(args, c.VarName)
		eq = append(eq, fmt.Sprintf("%q: %s", c.Name, c.VarName))
	}
	return viewFinder{
		Name:       name,
		Columns:    cols,
		Params:     strings.Join(params, ", "),
		Args:       strings.Join(args, ", "),
		EqMap:      strings.Join(eq, ", "),
		Private:    lowerFirst(name),
		PrivateTx:  lowerFirst(name) + "Tx",
		PrivateArg: strings.Join(args, ", "),
	}
}

func shouldInsert(c model.Column) bool {
	if c.IsAutoIncr || c.IsGenerated {
		return false
	}
	return true
}

func shouldUpdate(c model.Column) bool {
	if c.IsPrimaryKey || c.IsAutoIncr || c.IsGenerated {
		return false
	}
	if strings.EqualFold(c.Name, "created_at") || strings.EqualFold(c.Name, "create_time") {
		return false
	}
	return true
}

func sameColumns(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findViewColumn(cols []viewColumn, name string) (viewColumn, bool) {
	for _, c := range cols {
		if c.Name == name {
			return c, true
		}
	}
	return viewColumn{}, false
}

func joinGoNames(cols []viewColumn) string {
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		parts = append(parts, c.GoName)
	}
	return strings.Join(parts, "And")
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func snakeFileName(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "-", "_"))
}

func cleanImportPath(modulePath, dir string) string {
	dir = filepath.ToSlash(filepath.Clean(dir))
	dir = strings.TrimPrefix(dir, "./")
	if dir == "." || dir == "" {
		return modulePath
	}
	return strings.TrimRight(modulePath, "/") + "/" + strings.TrimLeft(dir, "/")
}

func writeTemplate(path string, tmpl string, ctx templateCtx, opt Options, res *Result) error {
	if !opt.Overwrite {
		if _, err := os.Stat(path); err == nil {
			res.Skipped = append(res.Skipped, path)
			return nil
		}
	}
	funcs := template.FuncMap{
		"domainImport": func() string { return cleanImportPath(opt.ModulePath, opt.DomainDir) },
		"repoImport":   func() string { return cleanImportPath(opt.ModulePath, opt.RepositoryDir) },
		"quote":        func(s string) string { return fmt.Sprintf("%q", s) },
	}
	t, err := template.New(filepath.Base(path)).Funcs(funcs).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", path, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, ctx); err != nil {
		return fmt.Errorf("execute template %s: %w", path, err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("format generated file %s: %w\n%s", path, err, buf.String())
	}
	if opt.DryRun {
		res.Written = append(res.Written, path)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return err
	}
	res.Written = append(res.Written, path)
	return nil
}
