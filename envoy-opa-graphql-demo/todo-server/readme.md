# todo-server 联调说明

## 端到端验证（创建 Todo -> Kafka 事件 -> Router 订阅）

以下步骤用于验证 `createTodo` 触发 Kafka 事件，并由 Cosmo Router 的 `employeeTodoChanged` 订阅收到更新。

### 1) 启动依赖服务

```bash
cd /workspace/envoy-opa-graphql-rewriter-demo/envoy-opa-graphql-demo
make compose-router-config
docker-compose up -d --build
```

### 2) 终端 A：建立 Router 订阅（SSE / SSE_POST）

#### 方式 A：SSE_POST（推荐）

```bash
curl -N http://localhost:3002/graphql \
  -H 'Accept: text/event-stream' \
  -H 'Content-Type: application/json' \
  -d '{"query":"subscription($employeeID: ID!){ employeeTodoChanged(employeeID:$employeeID){ id name todos { id content updatedAt deleted } } }","variables":{"employeeID":"emp-1"}}'
```

#### 方式 B：SSE（GET）

```bash
curl -N -G http://localhost:3002/graphql \
  -H 'Accept: text/event-stream' \
  --data-urlencode 'query=subscription($employeeID: ID!){ employeeTodoChanged(employeeID:$employeeID){ id name todos { id content updatedAt deleted } } }' \
  --data-urlencode 'variables={"employeeID":"emp-1"}'
```

### 3) 终端 B：监听 Kafka 事件

```bash
docker-compose exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server kafka:9092 \
  --topic employee.todo.events \
  --timeout-ms 30000 \
  --max-messages 1
```

### 4) 终端 C：调用 createTodo

```bash
curl -s http://localhost:3002/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"mutation($employeeID:ID!,$content:String!){ createTodo(employeeID:$employeeID, content:$content){ id employeeID content updatedAt deleted } }","variables":{"employeeID":"emp-1","content":"e2e-'$(date +%s)'"}}' | jq .
```

### 5) 预期结果

- 终端 C 返回创建成功的 Todo。
- 终端 B 收到 1 条 `employee.todo.events` 事件（JSON）。
- 终端 A（SSE 或 SSE_POST）收到 `employeeTodoChanged` 事件，且 `employee.id = "emp-1"`，`todos` 包含新建项。
