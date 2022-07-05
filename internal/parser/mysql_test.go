package parser

import "testing"

func TestParseMySQLDDL(t *testing.T) {
	ddl := `CREATE TABLE ` + "`" + `user` + "`" + ` (
  ` + "`" + `id` + "`" + ` bigint unsigned NOT NULL AUTO_INCREMENT,
  ` + "`" + `email` + "`" + ` varchar(128) NOT NULL,
  ` + "`" + `avatar_url` + "`" + ` varchar(255) DEFAULT NULL,
  ` + "`" + `created_at` + "`" + ` datetime NOT NULL,
  PRIMARY KEY (` + "`" + `id` + "`" + `),
  UNIQUE KEY ` + "`" + `uk_email` + "`" + ` (` + "`" + `email` + "`" + `)
);`
	tables, err := ParseMySQLDDL(ddl)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(tables))
	}
	got := tables[0]
	if got.Name != "user" || got.GoName != "User" {
		t.Fatalf("unexpected table: %+v", got)
	}
	if got.PrimaryKey == nil || len(got.PrimaryKey.Columns) != 1 || got.PrimaryKey.Columns[0] != "id" {
		t.Fatalf("unexpected primary key: %+v", got.PrimaryKey)
	}
	if len(got.UniqueKeys) != 1 || got.UniqueKeys[0].Columns[0] != "email" {
		t.Fatalf("unexpected unique keys: %+v", got.UniqueKeys)
	}
	if got.Columns[2].GoType != "*string" {
		t.Fatalf("expected nullable varchar as *string, got %s", got.Columns[2].GoType)
	}
	if got.Columns[3].GoType != "time.Time" {
		t.Fatalf("expected datetime as time.Time, got %s", got.Columns[3].GoType)
	}
}
