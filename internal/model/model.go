package model

// Table contains the parsed metadata for a SQL table.
type Table struct {
	Name       string
	GoName     string
	Columns    []Column
	PrimaryKey *Index
	UniqueKeys []Index
	NormalKeys []Index
}

// Column contains parsed metadata for a SQL column.
type Column struct {
	Name         string
	GoName       string
	VarName      string
	DBType       string
	BaseDBType   string
	GoType       string
	Nullable     bool
	Unsigned     bool
	IsPrimaryKey bool
	IsAutoIncr   bool
	HasDefault   bool
	IsGenerated  bool
	Comment      string
	JSONTag      string
	NeedsTime    bool
	NeedsSQLNull bool
}

// Index contains parsed metadata for an index.
type Index struct {
	Name      string
	Columns   []string
	GoName    string
	IsUnique  bool
	IsPrimary bool
}
