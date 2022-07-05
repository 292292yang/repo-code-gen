package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"repo-code-gen/internal/parser"
)

func TestGenerate(t *testing.T) {
	ddl := `CREATE TABLE user (
  id bigint NOT NULL AUTO_INCREMENT,
  email varchar(128) NOT NULL,
  name varchar(64) NOT NULL,
  created_at datetime NOT NULL,
  updated_at datetime NOT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_email (email)
);`
	tables, err := parser.ParseMySQLDDL(ddl)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	res, err := Generate(tables, Options{
		ModulePath:            "github.com/acme/app",
		DomainDir:             filepath.Join(dir, "internal/domain"),
		RepositoryDir:         filepath.Join(dir, "internal/repository"),
		ImplDir:               filepath.Join(dir, "internal/repositoryimpl/mysql"),
		DomainPackage:         "domain",
		RepositoryPackage:     "repository",
		ImplPackage:           "mysql",
		GenerateDomain:        true,
		GenerateInterface:     true,
		GenerateDelete:        true,
		GenerateUniqueFinders: true,
		Overwrite:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 3 {
		t.Fatalf("expected 3 files, got %d: %+v", len(res.Written), res.Written)
	}
	impl, err := os.ReadFile(filepath.Join(dir, "internal/repositoryimpl/mysql/user_repository_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(impl), "FindByEmail") {
		t.Fatalf("expected unique finder in generated impl:\n%s", string(impl))
	}
}
