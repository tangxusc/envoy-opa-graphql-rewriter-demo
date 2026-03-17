# demo-opa-graphql

一个最小可运行 Demo，展示：

- Go 服务接入 GraphQL (`gqlgen`)
- Bearer JWT 认证
- OPA (Rego) 进行字段级授权

## 关键约束

- Go 二进制固定为: `/Users/tangxu/sdk/go1.16rc1/bin/go`
- 包 sum 校验关闭: `GOSUMDB=off`
- 代理端口: `7890`

这些都已封装在 `Makefile`。

## 快速启动

```bash
cd demo-opa-graphql
make init
make generate
make test
make run
```

服务默认监听 `:8080`。

- Playground: `http://127.0.0.1:8080/`
- GraphQL Endpoint: `http://127.0.0.1:8080/query`

## 示例请求

### 1) 匿名访问公开字段（允许）

```bash
curl -s http://127.0.0.1:8080/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ publicInfo }"}'
```

### 2) 匿名访问 `me`（拒绝，unauthenticated）

```bash
curl -s http://127.0.0.1:8080/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ me { id name roles } }"}'
```

### 3) `user` 角色调用 `createPost`（拒绝，insufficient role）

先从服务启动日志中复制 `demo user token`，再执行：

```bash
curl -s http://127.0.0.1:8080/query \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer <USER_TOKEN>" \
  -d '{"query":"mutation { createPost(title:\"Hello\") { id title authorID } }"}'
```

### 4) `admin` 角色调用 `createPost`（允许）

先从服务启动日志中复制 `demo admin token`，再执行：

```bash
curl -s http://127.0.0.1:8080/query \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -d '{"query":"mutation { createPost(title:\"Hello\") { id title authorID } }"}'
```

## 授权策略

策略文件: `policy/authz.rego`

输入形态：

- `input.user.authenticated`
- `input.user.subject`
- `input.user.roles`
- `input.operation`
- `input.field`
- `input.args`
