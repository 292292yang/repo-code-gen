# repo-code-gen

`repo-code-gen` 是一个面向 go-zero 项目的 Repository 代码生成工具。

它不会生成 go-zero 原生 `model`，而是根据 MySQL `CREATE TABLE` 建表语句生成：

- `domain` 实体结构体
- `repository` 接口
- `repositoryimpl/mysql` 实现
- 普通 CRUD 方法
- 事务 CRUD 方法，事务参数使用 `sqlx.Session`
- `UNIQUE KEY` 对应的 `FindByXxx` 查询方法

生成出来的 SQL 拼装使用 [Masterminds/squirrel](https://github.com/Masterminds/squirrel)，执行层使用 go-zero 的 `github.com/zeromicro/go-zero/core/stores/sqlx`。

## 生成代码风格

以 `user` 表为例，会生成类似下面的接口：

```go
type UserRepository interface {
    Create(ctx context.Context, entity *domain.User) error
    CreateTx(ctx context.Context, session sqlx.Session, entity *domain.User) error

    FindByID(ctx context.Context, id int64) (*domain.User, error)
    FindByIDTx(ctx context.Context, session sqlx.Session, id int64) (*domain.User, error)

    FindByEmail(ctx context.Context, email string) (*domain.User, error)
    FindByEmailTx(ctx context.Context, session sqlx.Session, email string) (*domain.User, error)

    Update(ctx context.Context, entity *domain.User) error
    UpdateTx(ctx context.Context, session sqlx.Session, entity *domain.User) error

    Delete(ctx context.Context, id int64) error
    DeleteTx(ctx context.Context, session sqlx.Session, id int64) error
}
```

生成的实现采用下面的复用方式：

```text
Create   -> private create(ctx, session, entity) -> sqlx.SqlConn
CreateTx -> private create(ctx, session, entity) -> sqlx.Session
```

也就是说，普通方法和事务方法不会重复拼 SQL。

## 安装

在本工程目录执行：

```bash
go build -o bin/repo-code-gen ./cmd/repo-code-gen
```

然后把 `bin/repo-code-gen` 放到你的 PATH 中，或者直接用完整路径执行。

也可以直接运行：

```bash
go run ./cmd/repo-code-gen mysql -src examples/user.sql -module github.com/acme/demo
```

## 快速开始

假设你的 go-zero 项目模块名是：

```text
github.com/acme/demo
```

你的 SQL 文件在：

```text
deploy/sql/user.sql
```

执行：

```bash
repo-code-gen mysql \
  -src ./deploy/sql/user.sql \
  -module github.com/acme/demo
```

默认会生成到：

```text
out/domain/user_gen.go
out/repository/user_repository_gen.go
out/repositoryimpl/mysql/user_repository_gen.go
```

如果当前目录或父目录有 `go.mod`，可以省略 `-module`，工具会自动读取最近的 `go.mod` 中的 `module`。

## 批量生成

`-src` 可以是目录。工具会递归读取目录下所有 `.sql` 文件：

```bash
repo-code-gen mysql -src ./deploy/sql
```

只生成指定表：

```bash
repo-code-gen mysql -src ./deploy/sql -table user,order
```

## 自定义输出目录

```bash
repo-code-gen mysql \
  -src ./deploy/sql/user.sql \
  -module github.com/acme/demo \
  -domain-dir internal/domain \
  -repo-dir internal/repository \
  -impl-dir internal/repositoryimpl/mysql
```

## 事务使用示例

在 go-zero 业务逻辑中，可以这样使用生成的 `Tx` 方法：

```go
err := conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
    if err := userRepo.CreateTx(ctx, session, user); err != nil {
        return err
    }

    if err := orderRepo.CreateTx(ctx, session, order); err != nil {
        return err
    }

    return nil
})
```

## 在 ServiceContext 中注入

```go
package svc

import (
    "github.com/zeromicro/go-zero/core/stores/sqlx"

    mysqlrepo "github.com/acme/demo/internal/repositoryimpl/mysql"
)

type ServiceContext struct {
    UserRepo repository.UserRepository
}

func NewServiceContext(c config.Config) *ServiceContext {
    conn := sqlx.NewMysql(c.Mysql.DataSource)

    return &ServiceContext{
        UserRepo: mysqlrepo.NewUserRepository(conn),
    }
}
```

## 命令参数

```text
repo-code-gen mysql flags:

-src string
    SQL 文件或 SQL 目录，必填。

-table string
    只生成指定表，多个表用英文逗号分隔，例如 user,order。

-module string
    目标项目 Go module path。省略时自动从当前目录或父目录 go.mod 读取。

-domain-dir string
    domain 结构体输出目录，默认 internal/domain。

-repo-dir string
    repository 接口输出目录，默认 internal/repository。

-impl-dir string
    MySQL repository 实现输出目录，默认 internal/repositoryimpl/mysql。

-domain-package string
    domain 文件 package 名，默认 domain。

-repo-package string
    repository 文件 package 名，默认 repository。

-impl-package string
    MySQL 实现文件 package 名，默认 mysql。

-generate-domain bool
    是否生成 domain struct，默认 true。

-generate-interface bool
    是否生成 repository interface，默认 true。

-generate-delete bool
    是否生成 Delete/DeleteTx，默认 true。

-generate-unique-finders bool
    是否根据 UNIQUE KEY 生成 FindByXxx，默认 true。

-overwrite bool
    是否覆盖已存在的生成文件，默认 true。

-dry-run bool
    只打印将要生成的文件路径，不写入文件。
```

## 字段生成规则

### Insert

默认参与 `Create/CreateTx` 的字段：

- 非 `AUTO_INCREMENT` 字段
- 非生成列字段

### Update

默认参与 `Update/UpdateTx` 的字段：

- 排除主键
- 排除 `AUTO_INCREMENT` 字段
- 排除生成列字段
- 排除 `created_at` / `create_time`

### FindBy

默认生成：

- 主键查询：`FindByID` / `FindByIDTx`
- 唯一索引查询：`FindByEmail`、`FindByTenantIDAndEmail` 等

### Delete

默认生成物理删除：

```sql
DELETE FROM table WHERE id = ?
```

如果你需要软删除，可以先关闭默认删除：

```bash
repo-code-gen mysql -src ./deploy/sql/user.sql -generate-delete=false
```

然后在手写 repository 扩展文件中自行实现软删除方法。

## MySQL 类型映射

| MySQL 类型 | Go 类型 |
|---|---|
| bigint/int/smallint/mediumint/year | int64 |
| unsigned integer | uint64 |
| tinyint(1), bool, boolean | bool |
| float/double/decimal/numeric | float64 |
| char/varchar/text/json/enum/set | string |
| date/datetime/timestamp/time | time.Time |
| blob/binary/varbinary | []byte |

可空字段默认生成指针类型，例如：

```go
AvatarURL *string `db:"avatar_url" json:"avatar_url"`
```

## 注意事项

1. 第一版只支持 MySQL `CREATE TABLE`。
2. 每张表必须有且只有一个主键字段。
3. 生成文件后缀为 `_gen.go`，可以重复生成覆盖。
4. 工具本身只使用 Go 标准库；生成出来的代码需要你的业务项目引入：

```bash
go get github.com/Masterminds/squirrel
go get github.com/zeromicro/go-zero
```

## 示例

执行：

```bash
make example
```

会根据 `examples/user.sql` 生成示例代码到：

```text
_generated/internal/domain
_generated/internal/repository
_generated/internal/repositoryimpl/mysql
```

## 当前限制

- 暂不支持 PostgreSQL。
- 暂不支持复合主键。
- 暂不支持自动生成分页查询。
- 暂不生成 cache 层。
- DDL 解析器覆盖常见 MySQL 建表语句，极复杂的表达式、外键约束或特殊生成列可能需要后续增强。
