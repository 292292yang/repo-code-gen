package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"repo-code-gen/internal/generator"
	"repo-code-gen/internal/model"
	"repo-code-gen/internal/parser"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage()
		return nil
	}
	if args[0] == "version" || args[0] == "--version" || args[0] == "-v" {
		fmt.Println("repo-code-gen", version)
		return nil
	}
	switch args[0] {
	case "mysql":
		return runMySQL(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runMySQL(args []string) error {
	fs := flag.NewFlagSet("mysql", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var src string
	var tableFilter string
	var modulePath string
	var domainDir string
	var repoDir string
	var implDir string
	var domainPkg string
	var repoPkg string
	var implPkg string
	var generateDomain bool
	var generateInterface bool
	var generateDelete bool
	var generateUnique bool
	var overwrite bool
	var dryRun bool

	fs.StringVar(&src, "src", "", "SQL file or directory containing .sql files")
	fs.StringVar(&tableFilter, "table", "", "optional comma-separated table names to generate")
	fs.StringVar(&modulePath, "module", "", "target Go module path; defaults to nearest go.mod module")
	fs.StringVar(&domainDir, "domain-dir", "out/domain", "output directory for generated domain structs")
	fs.StringVar(&repoDir, "repo-dir", "out/repository", "output directory for generated repository interfaces")
	fs.StringVar(&implDir, "impl-dir", "out/repositoryimpl/mysql", "output directory for generated MySQL repository implementations")
	fs.StringVar(&domainPkg, "domain-package", "domain", "package name for generated domain structs")
	fs.StringVar(&repoPkg, "repo-package", "repository", "package name for generated repository interfaces")
	fs.StringVar(&implPkg, "impl-package", "mysql", "package name for generated MySQL implementations")
	fs.BoolVar(&generateDomain, "generate-domain", true, "generate domain struct files")
	fs.BoolVar(&generateInterface, "generate-interface", true, "generate repository interface files")
	fs.BoolVar(&generateDelete, "generate-delete", true, "generate Delete/DeleteTx methods")
	fs.BoolVar(&generateUnique, "generate-unique-finders", true, "generate FindBy... methods for UNIQUE KEY indexes")
	fs.BoolVar(&overwrite, "overwrite", true, "overwrite generated files")
	fs.BoolVar(&dryRun, "dry-run", false, "print what would be generated without writing files")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if src == "" {
		return errors.New("-src is required")
	}
	if modulePath == "" {
		var err error
		modulePath, err = detectModulePath()
		if err != nil {
			return fmt.Errorf("-module not provided and go.mod could not be detected: %w", err)
		}
	}

	sqlText, err := readSQLSource(src)
	if err != nil {
		return err
	}
	tables, err := parser.ParseMySQLDDL(sqlText)
	if err != nil {
		return err
	}
	tables = filterTables(tables, tableFilter)
	if len(tables) == 0 {
		return fmt.Errorf("no tables matched filter %q", tableFilter)
	}

	res, err := generator.Generate(tables, generator.Options{
		ModulePath:            modulePath,
		DomainDir:             domainDir,
		RepositoryDir:         repoDir,
		ImplDir:               implDir,
		DomainPackage:         domainPkg,
		RepositoryPackage:     repoPkg,
		ImplPackage:           implPkg,
		GenerateDomain:        generateDomain,
		GenerateInterface:     generateInterface,
		GenerateDelete:        generateDelete,
		GenerateUniqueFinders: generateUnique,
		Overwrite:             overwrite,
		DryRun:                dryRun,
	})
	if err != nil {
		return err
	}

	verb := "generated"
	if dryRun {
		verb = "would generate"
	}
	for _, path := range res.Written {
		fmt.Printf("%s %s\n", verb, path)
	}
	for _, path := range res.Skipped {
		fmt.Printf("skipped %s\n", path)
	}
	return nil
}

func readSQLSource(src string) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		data, err := os.ReadFile(src)
		return string(data), err
	}
	var b strings.Builder
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".sql") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.WriteString("\n-- file: ")
		b.WriteString(path)
		b.WriteString("\n")
		b.Write(data)
		b.WriteString("\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func filterTables(tables []model.Table, filter string) []model.Table {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return tables
	}
	wanted := map[string]bool{}
	for _, part := range strings.Split(filter, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			wanted[part] = true
		}
	}
	out := make([]model.Table, 0, len(tables))
	for _, t := range tables {
		if wanted[t.Name] {
			out = append(out, t)
		}
	}
	return out
}

func detectModulePath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		path := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(path)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
				}
			}
			return "", fmt.Errorf("module directive not found in %s", path)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func printUsage() {
	fmt.Print(`repo-code-gen generates go-zero repository CRUD code from MySQL CREATE TABLE SQL.

Usage:
  repo-code-gen mysql -src ./deploy/sql/user.sql -module github.com/acme/app
  repo-code-gen mysql -src ./deploy/sql -table user,order

Commands:
  mysql       generate repository code from MySQL DDL
  version     print version

Run "repo-code-gen mysql -h" for flags.
`)
}
