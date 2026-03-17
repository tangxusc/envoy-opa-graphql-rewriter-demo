.PHONY: up down build logs gen-token test-anon test-admin test-user clean

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

clean:
	docker compose down --rmi all --volumes
