.PHONY: up down build logs gen-token compose-router-config test-anon test-admin test-user test-mutation test-subscribe-admin test-subscribe-user test-federation-todos clean

up: build
	docker compose up -d

down:
	docker compose down

build:
	docker compose build

logs:
	docker compose logs -f

gen-token:
	cd authz-server && go run cmd/gen-token/main.go

compose-router-config:
	npx -y wgc@latest router compose --input ./cosmo/graph.yaml --out ./cosmo/config.json

test-anon:
	@echo "=== 测试匿名请求 (应返回 403) ==="
	curl -s http://localhost:10000/query \
		-H "Content-Type: application/json" \
		-d '{"query":"{ employeeByID(id:\"emp-1\") { id name salary } }"}' | jq .

test-admin:
	@echo "=== 测试 admin 请求 (应返回含 salary) ==="
	@echo "请先运行 make gen-token 获取 token，然后执行:"
	@echo 'curl -s http://localhost:10000/query -H "Content-Type: application/json" -H "Authorization: Bearer <ADMIN_TOKEN>" -d '"'"'{"query":"{ employeeByID(id:\"emp-1\") { id name salary } }"}'"'"' | jq .'

test-user:
	@echo "=== 测试普通用户请求 (salary 应被过滤) ==="
	@echo "请先运行 make gen-token 获取 token，然后执行:"
	@echo 'curl -s http://localhost:10000/query -H "Content-Type: application/json" -H "Authorization: Bearer <USER_TOKEN>" -d '"'"'{"query":"{ employeeByID(id:\"emp-1\") { id name salary } }"}'"'"' | jq .'

test-mutation:
	@echo "=== 测试 Mutation (更新员工) ==="
	@echo "请先运行 make gen-token 获取 admin token，然后执行:"
	@echo 'curl -s http://localhost:10000/query -H "Content-Type: application/json" -H "Authorization: Bearer <ADMIN_TOKEN>" -d '"'"'{"query":"mutation { updateEmployee(id: \"emp-1\", name: \"Alice Updated\") { id name salary } }"}'"'"' | jq .'

test-subscribe-admin:
	@echo "=== 测试 Admin SSE Subscription (应包含 salary) ==="
	@echo "请先运行 make gen-token 获取 admin token，然后执行:"
	@echo 'curl -N -H "Accept: text/event-stream" -H "Authorization: Bearer <ADMIN_TOKEN>" -H "Content-Type: application/json" -d '"'"'{"query":"subscription { employeeUpdated { id name salary } }"}'"'"' http://localhost:10000/query'

test-subscribe-user:
	@echo "=== 测试普通用户 SSE Subscription (salary 应被过滤) ==="
	@echo "请先运行 make gen-token 获取 user token，然后执行:"
	@echo 'curl -N -H "Accept: text/event-stream" -H "Authorization: Bearer <USER_TOKEN>" -H "Content-Type: application/json" -d '"'"'{"query":"subscription { employeeUpdated { id name salary } }"}'"'"' http://localhost:10000/query'

test-federation-todos:
	@echo "=== 测试联邦 Employee.todos 查询 ==="
	@echo 'curl -s http://localhost:10000/query -H "Content-Type: application/json" -d '"'"'{"query":"{ employeeByID(id:\"emp-1\") { id name todos { id content updatedAt deleted } } }"}'"'"' | jq .'

clean:
	docker compose down --rmi all --volumes
