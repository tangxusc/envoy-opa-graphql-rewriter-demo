# GraphQL Subscription 支持情况分析

## 当前状态：不支持 ❌

当前 envoy-opa-graphql-demo 项目 **不支持 GraphQL Subscription**。

---

## 详细分析

### 1. Schema 层面

GraphQL Schema (`graphql-server/graph/schema.graphqls`) 仅定义了 `Query` 类型，无 `Subscription` 和 `Mutation`：

```graphql
type Query {
  employeeByID(id: String!): Employee
  employees: [Employee!]!
}
```

### 2. GraphQL Server 层面

- **框架**: gqlgen v0.17.81（本身支持 Subscription）
- **问题**:
  - 未配置 WebSocket transport
  - 使用标准 HTTP `handler.NewDefaultServer()`，未添加 WebSocket 支持
  - `gorilla/websocket` 在 `go.mod` 中作为 gqlgen 的传递依赖存在，但未被实际使用
  - 服务器入口 (`cmd/server/main.go`) 仅注册了 HTTP handler，无 WebSocket 升级逻辑

### 3. Envoy 代理层面（最大障碍）

即使 GraphQL Server 添加了 Subscription 支持，当前 Envoy 配置也无法正确代理：

- `ext_authz` filter 基于 HTTP 请求/响应模型，处理 request body 中的 GraphQL query
- Lua filter 用于改写 HTTP request body，不适用于 WebSocket 帧
- 整个鉴权流程（JWT 解析 → 字段提取 → OPA 策略评估 → body 改写）假设的是普通 HTTP POST 请求
- 未配置 `upgrade_configs` 以支持 WebSocket 协议升级

### 4. Authz Server 层面

- 当前 `Check` 方法针对单次 HTTP 请求进行鉴权
- 不支持 WebSocket 连接初始化时的 `connection_init` 消息鉴权
- 无法处理长连接场景下的持续授权检查

### 5. OPA 策略层面

- 策略 (`policy/graphql_authz.rego`) 基于单次请求的输入进行评估
- 无订阅场景下的持续授权机制

---

## 改造方案

若要添加 Subscription 支持，需改造以下组件：

### Step 1: GraphQL Server

- [ ] 在 `schema.graphqls` 中定义 `Subscription` 类型
- [ ] 实现 Subscription resolver（基于 Go channel）
- [ ] 在 gqlgen 中启用 WebSocket transport（`graphql-ws` 或 `subscriptions-transport-ws` 协议）
- [ ] 配置 `handler.NewDefaultServer()` 添加 WebSocket 支持

### Step 2: Envoy 代理

- [ ] 在 `envoy.yaml` 中添加 `upgrade_configs` 配置，支持 WebSocket 协议升级
- [ ] 调整 `ext_authz` filter，使其在 WebSocket 握手阶段进行鉴权
- [ ] Lua filter 需适配或绕过 WebSocket 帧处理

### Step 3: Authz Server

- [ ] 支持 WebSocket 握手请求的鉴权（解析 `Upgrade: websocket` 头）
- [ ] 在连接初始化阶段（`connection_init`）完成 JWT 验证和角色提取
- [ ] 考虑是否需要对每个订阅消息进行细粒度授权

### Step 4: OPA 策略

- [ ] 扩展策略以支持订阅操作类型（`subscription`）
- [ ] 定义订阅场景下的字段级访问控制规则
- [ ] 考虑持续授权：Token 过期后是否断开订阅连接

---

## 技术参考

| 组件 | 当前版本 | 备注 |
|------|----------|------|
| gqlgen | v0.17.81 | 已内置 Subscription 支持，需启用 |
| gorilla/websocket | v1.5.0 | 已在依赖中，需实际引用 |
| Envoy | v1.31 | 支持 WebSocket，需配置 `upgrade_configs` |
| OPA | v1.14.1 | 策略需扩展 |

## 优先级建议

1. **高优先级**: GraphQL Server 添加 Subscription 支持（工作量最小，可独立验证）
2. **中优先级**: Envoy WebSocket 代理配置
3. **中优先级**: Authz Server 适配 WebSocket 鉴权
4. **低优先级**: OPA 策略扩展（可先用粗粒度连接级鉴权）
