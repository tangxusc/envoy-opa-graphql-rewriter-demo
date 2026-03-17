# graphql-rewiter

## 功能

GraphQL查询重写器，用于在查询执行前根据权限策略过滤字段。

## 输入

```graphql
type Employee {
  id: String!
  name: String!
  salary: Int!
}

query EmployeeSalaryWithFragment($id: String!) {
  employeeByID(id: $id) {
    id
    ...SalaryPart
  }
}

fragment SalaryPart on Employee {
  salary
}
```
权限决策输入:
```json
{
  "allow": true,
  "removed_fields": [
    "employeeByID.salary"
  ]
}
```

## 输出

```graphql
query EmployeeSalaryWithFragment($id: String!) {
  employeeByID(id: $id) {
    id
  }
}
```


## 功能详细介绍

程序主要根据graphql,权限决策数据json,生成过滤后的graphql

## Go 实现说明

已在当前目录实现：

- `rewriter.go`: 核心重写逻辑 `RewriteQuery(query, decisionJSON)`  
  - 解析 GraphQL AST
  - 根据 `removed_fields` 删除匹配路径字段（例如 `employeeByID.salary`）
  - 支持 `fragment spread` 与 `inline fragment`
  - 输出时会将 fragment 内联并移除 fragment 定义
- `main.go`: 命令行入口
- `rewriter_test.go`: 单元测试

## 运行方式

默认运行（启动 Web 页面）：

```bash
go run .
```

打开：`http://localhost:8080`

也可以显式指定：

```bash
go run . -serve -addr :8080
```

指定输入文件运行：

```bash
go run . -query ./query.graphql -decision ./decision.json
```

测试：

```bash
go test ./...
```

## HTTP 接口

`POST /api/rewrite`

请求 JSON：

```json
{
  "graphql": "query EmployeeSalaryWithFragment($id: String!) { employeeByID(id: $id) { id ...SalaryPart } } fragment SalaryPart on Employee { salary }",
  "decision": {
    "allow": true,
    "removed_fields": ["employeeByID.salary"]
  }
}
```

响应 JSON：

```json
{
  "rewritten_query": "query EmployeeSalaryWithFragment ($id: String!) { employeeByID(id: $id) { id } }"
}
```
