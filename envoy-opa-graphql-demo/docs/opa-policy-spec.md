# OPA Rego 策略规范

## 1. 概述

本项目使用 OPA (Open Policy Agent) Rego 策略对 GraphQL 请求实施字段级授权。

- **策略包名**: `graphqlapi.authz`
- **查询路径**: `data.graphqlapi.authz.decision`
- **策略文件**: `authz-server/policy/graphql_authz.rego`

### 授权流程

```
客户端请求 → Envoy → ext_authz gRPC (authz-server)
                          │
                          ├─ 1. 解析 JWT，提取用户信息（subject、roles、privileges）
                          ├─ 2. 将角色列表编码为 Bloom Filter 权限集合
                          ├─ 3. 解析 GraphQL body，提取字段列表和操作类型
                          ├─ 4. 构造 EvalInput，调用 OPA 评估
                          └─ 5. 根据 Decision 决定放行/拒绝/改写请求
```

- **放行 (allow=true, denied_fields 为空)**: 原样转发请求到 GraphQL 服务。
- **字段过滤 (allow=true, denied_fields 非空)**: 从 GraphQL query 中移除受限字段后转发。
- **拒绝 (allow=false)**: 返回 403 Forbidden 或 401 Unauthorized。

---

## 2. 输入规范 (input)

OPA 策略接收的输入是一个 `EvalInput` JSON 对象，包含 `user` 和 `request` 两部分。

### 完整 JSON 结构

```json
{
  "user": {
    "authenticated": true,
    "subject": "alice",
    "roles": ["admin"],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": "base64-encoded-bloom-filter-string..."
  },
  "request": {
    "query": "query { employees { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "query"
  }
}
```

### 字段说明

#### `user` (UserInput)

| 字段 | 类型 | JSON key | 说明 |
|------|------|----------|------|
| Authenticated | `bool` | `authenticated` | 用户是否已通过 JWT 认证 |
| Subject | `string` | `subject` | 用户标识（JWT sub 字段） |
| Roles | `[]string` | `roles` | 用户角色列表（JWT roles 字段） |
| CurrentTime | `string` | `current_time` | ISO 8601 (RFC 3339) 格式的当前 UTC 时间，由 authz-server 在评估时生成 |
| Privileges | `string` | `privileges` | Base64 编码的 Bloom Filter 权限集合，由 JWT 中的 `privileges` 字段提供 |

#### `request` (RequestInput)

| 字段 | 类型 | JSON key | 说明 |
|------|------|----------|------|
| Query | `string` | `query` | 原始 GraphQL 查询字符串 |
| Fields | `[]string` | `fields` | 从 query 中提取的顶层字段名列表 |
| OperationType | `string` | `operation_type` | GraphQL 操作类型：`"query"` / `"mutation"` / `"subscription"` / `""` |

---

## 3. 输出规范 (decision)

策略评估返回一个 `Decision` JSON 对象。

### 完整 JSON 结构

```json
{
  "allow": true,
  "denied_fields": ["salary"],
  "reason": ""
}
```

### 字段说明

| 字段 | 类型 | JSON key | 说明 |
|------|------|----------|------|
| Allow | `bool` | `allow` | 是否允许请求通过 |
| DeniedFields | `[]string` | `denied_fields` | 需要从 GraphQL query 中移除的字段列表 |
| Reason | `string` | `reason` | 拒绝原因（多个原因以 `; ` 分隔），allow=true 时为空字符串 |

### Decision 处理逻辑

| allow | denied_fields | 服务端行为 |
|-------|---------------|-----------|
| `true` | `[]` | 原样放行请求 |
| `true` | `["salary", ...]` | 从 body 中移除受限字段后放行（通过 `x-rewritten-body` header 传递改写后的 body） |
| `false` | — | 返回 HTTP 403 (Forbidden) 并附带 `reason` |

---

## 4. 权限模型

### 4.1 Bloom Filter 权限机制

本项目使用 Bloom Filter 对权限进行高效编码和检查，而非在策略中直接匹配角色名。

**工作原理：**

1. **编码阶段（JWT 生成时）**：根据用户的角色列表，从 `RolePrivileges` 映射表收集所有权限，构建 Bloom Filter，Base64 编码后存入 JWT 的 `privileges` 字段。
2. **检查阶段（OPA 评估时）**：策略通过自定义内置函数 `hasPrivilege(privileges_string, privilege_name)` 解码 Bloom Filter 并检查是否包含指定权限。

**Bloom Filter 参数：**

| 参数 | 值 | 说明 |
|------|------|------|
| 预估容量 | 1024 | `estimatedMaxPrivileges` |
| 误判率 | 0.01 (1%) | `falsePositiveRate` |

### 4.2 角色-权限映射

当前系统定义了以下角色及其权限（`authz-server/internal/privilege/bloom.go`）：

| 角色 | 权限列表 |
|------|----------|
| `admin` | `read:employee`, `write:employee`, `delete:employee`, `read:salary`, `write:salary`, `read:department`, `write:department`, `manage:users`, `manage:roles` |
| `user` | `read:employee`, `read:department` |
| `hr` | `read:employee`, `write:employee`, `read:salary`, `write:salary`, `read:department` |

### 4.3 自定义内置函数 `hasPrivilege`

在 OPA 评估器初始化时注册的自定义函数：

```
hasPrivilege(privileges_string: string, privilege_name: string) -> bool
```

- **参数**：
  - `privileges_string`: Base64 编码的 Bloom Filter 字符串（来自 `input.user.privileges`）
  - `privilege_name`: 要检查的权限名称（如 `"read:salary"`）
- **返回**：`true` 表示权限存在（可能有 1% 误判率），`false` 表示权限不存在
- **实现**：解码 Base64 → 反序列化 Bloom Filter → 调用 `TestString()` 检查

---

## 5. 策略规则说明

以下是当前 `graphql_authz.rego` 中定义的规则。

### 5.1 默认值

```rego
default allow := false
default can_read_salary := false
```

- 所有请求默认被拒绝（deny by default）。
- 薪资读取权限默认关闭。

### 5.2 认证检查

```rego
deny contains "unauthenticated request" if {
    not input.user.authenticated
}
```

未认证请求会被加入 `deny` 集合，导致 `allow = false`。

### 5.3 权限检查 — 薪资字段

```rego
can_read_salary if {
    hasPrivilege(input.user.privileges, "read:salary")
}
```

通过 Bloom Filter 自定义函数 `hasPrivilege` 检查用户是否拥有 `read:salary` 权限。根据当前角色-权限映射，`admin` 和 `hr` 角色拥有此权限。

### 5.4 字段过滤

```rego
denied_fields contains "salary" if {
    input.user.authenticated
    not can_read_salary
}
```

对于已认证但不具备 `read:salary` 权限的用户，`salary` 会被加入 `denied_fields`。

### 5.5 最终决策构造

```rego
decision := {
    "allow": count(deny) == 0,
    "denied_fields": denied_fields,
    "reason": concat("; ", deny),
}
```

- `allow`: 当 `deny` 集合为空时为 `true`。
- `denied_fields`: 所有被拒绝字段的集合。
- `reason`: `deny` 集合中所有拒绝原因的拼接。

---

## 6. GraphQL 操作支持

### 当前 Schema 定义的操作

| 操作类型 | 操作 | 说明 |
|----------|------|------|
| `query` | `employeeByID(id: String!): Employee` | 按 ID 查询员工 |
| `query` | `employees: [Employee!]!` | 查询所有员工 |
| `mutation` | `updateEmployee(id: String!, name: String, salary: Int): Employee!` | 更新员工信息 |
| `subscription` | `employeeUpdated(id: String): Employee!` | 订阅员工更新事件 |

### Employee 类型字段

| 字段 | 类型 | 受权限控制 |
|------|------|-----------|
| `id` | `String!` | 否 |
| `name` | `String!` | 否 |
| `salary` | `Int!` | 是（需要 `read:salary` 权限，`admin` 和 `hr` 角色拥有此权限） |

### 各操作类型的授权行为

当前策略对 `query`、`mutation`、`subscription` **统一适用**相同的授权规则：

1. **认证检查**: 所有操作类型均要求 JWT 认证。
2. **字段过滤**: `salary` 字段的访问限制适用于所有操作类型（包括 mutation 返回值和 subscription 推送数据中的 salary 字段）。
3. **操作类型限制**: 当前策略未按 `operation_type` 做差异化授权（`operation_type` 已传入 input，可在策略中使用）。

---

## 7. 示例

### 7.1 未认证用户

**Input:**

```json
{
  "user": {
    "authenticated": false,
    "subject": "",
    "roles": [],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": ""
  },
  "request": {
    "query": "query { employees { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "query"
  }
}
```

**Decision:**

```json
{
  "allow": false,
  "denied_fields": [],
  "reason": "unauthenticated request"
}
```

**HTTP 响应**: `401 Unauthorized`

---

### 7.2 admin 用户 — Query

**Input:**

```json
{
  "user": {
    "authenticated": true,
    "subject": "alice",
    "roles": ["admin"],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": "<base64-bloom-filter-with-read:salary>"
  },
  "request": {
    "query": "query { employees { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "query"
  }
}
```

**Decision:**

```json
{
  "allow": true,
  "denied_fields": [],
  "reason": ""
}
```

**行为**: 请求原样转发，返回包含 `salary` 的完整数据。`admin` 角色拥有 `read:salary` 权限，Bloom Filter 检查通过。

---

### 7.3 普通用户 (user 角色) — Query

**Input:**

```json
{
  "user": {
    "authenticated": true,
    "subject": "bob",
    "roles": ["user"],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": "<base64-bloom-filter-without-read:salary>"
  },
  "request": {
    "query": "query { employees { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "query"
  }
}
```

**Decision:**

```json
{
  "allow": true,
  "denied_fields": ["salary"],
  "reason": ""
}
```

**行为**: `salary` 字段从 query 中移除，改写后的 body 通过 `x-rewritten-body` header 传递，GraphQL 服务只返回 `id` 和 `name`。`user` 角色不拥有 `read:salary` 权限。

---

### 7.4 普通用户 (user 角色) — Mutation

**Input:**

```json
{
  "user": {
    "authenticated": true,
    "subject": "bob",
    "roles": ["user"],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": "<base64-bloom-filter-without-read:salary>"
  },
  "request": {
    "query": "mutation { updateEmployee(id: \"1\", name: \"Bob\") { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "mutation"
  }
}
```

**Decision:**

```json
{
  "allow": true,
  "denied_fields": ["salary"],
  "reason": ""
}
```

**行为**: mutation 本身被允许，但返回字段中的 `salary` 被移除。

---

### 7.5 admin 用户 — Subscription

**Input:**

```json
{
  "user": {
    "authenticated": true,
    "subject": "alice",
    "roles": ["admin"],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": "<base64-bloom-filter-with-read:salary>"
  },
  "request": {
    "query": "subscription { employeeUpdated { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "subscription"
  }
}
```

**Decision:**

```json
{
  "allow": true,
  "denied_fields": [],
  "reason": ""
}
```

**行为**: 订阅请求原样转发，推送数据包含 `salary`。

---

### 7.6 普通用户 (user 角色) — Subscription

**Input:**

```json
{
  "user": {
    "authenticated": true,
    "subject": "bob",
    "roles": ["user"],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": "<base64-bloom-filter-without-read:salary>"
  },
  "request": {
    "query": "subscription { employeeUpdated { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "subscription"
  }
}
```

**Decision:**

```json
{
  "allow": true,
  "denied_fields": ["salary"],
  "reason": ""
}
```

**行为**: 订阅请求中 `salary` 字段被移除。

---

### 7.7 hr 用户 — Query（可查看薪资）

**Input:**

```json
{
  "user": {
    "authenticated": true,
    "subject": "carol",
    "roles": ["hr"],
    "current_time": "2026-03-18T10:30:00Z",
    "privileges": "<base64-bloom-filter-with-read:salary>"
  },
  "request": {
    "query": "query { employees { id name salary } }",
    "fields": ["id", "name", "salary"],
    "operation_type": "query"
  }
}
```

**Decision:**

```json
{
  "allow": true,
  "denied_fields": [],
  "reason": ""
}
```

**行为**: `hr` 角色拥有 `read:salary` 权限，请求原样转发，返回包含 `salary` 的完整数据。

---

## 8. 扩展指南

### 8.1 添加新权限

要添加新的字段权限控制，需要完成以下步骤：

**1. 在角色-权限映射中添加权限（`authz-server/internal/privilege/bloom.go`）：**

```go
var RolePrivileges = map[string][]string{
    "admin": {
        // ... 现有权限 ...
        "read:email",  // 新增
    },
    "hr": {
        // ... 现有权限 ...
        "read:email",  // 新增
    },
}
```

**2. 在 Rego 策略中添加权限检查规则：**

```rego
default can_read_email := false

can_read_email if {
    hasPrivilege(input.user.privileges, "read:email")
}

denied_fields contains "email" if {
    input.user.authenticated
    not can_read_email
}
```

**3. 在 GraphQL Schema 中添加 `email` 字段。**

### 8.2 添加新角色

在 `RolePrivileges` 映射中添加新角色及其权限列表：

```go
var RolePrivileges = map[string][]string{
    // ... 现有角色 ...
    "manager": {
        "read:employee", "write:employee",
        "read:salary",
        "read:department",
    },
}
```

新角色的权限会自动编码到 Bloom Filter 中，策略中的 `hasPrivilege` 检查无需修改。

### 8.3 按操作类型限制

当前策略未区分操作类型，可通过 `input.request.operation_type` 添加限制。例如只允许拥有 `write:employee` 权限的用户执行 mutation：

```rego
deny contains "mutation not allowed" if {
    input.request.operation_type == "mutation"
    not hasPrivilege(input.user.privileges, "write:employee")
}
```

或禁止所有用户使用 subscription：

```rego
deny contains "subscription not supported" if {
    input.request.operation_type == "subscription"
}
```

### 8.4 基于具体操作名限制

可解析 `input.request.query` 中的操作名进行更细粒度的控制。例如限制 `updateEmployee` 仅拥有 `write:employee` 权限的用户可调用：

```rego
deny contains "updateEmployee requires write:employee" if {
    input.request.operation_type == "mutation"
    contains(input.request.query, "updateEmployee")
    not hasPrivilege(input.user.privileges, "write:employee")
}
```

### 8.5 添加字段值级权限（高级）

如需根据数据值（如"只能查看本人薪资"）做授权，需要在 input 中传递更多上下文（如查询的目标员工 ID），并在策略中比对 `input.user.subject`。
