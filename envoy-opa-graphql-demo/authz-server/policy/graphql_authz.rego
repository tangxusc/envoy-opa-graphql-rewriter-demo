package graphqlapi.authz

import rego.v1

# 默认值
default allow := false
default can_read_salary := false

# 未认证用户直接拒绝
deny contains "unauthenticated request" if {
	not input.user.authenticated
}

# 管理员可读取任意员工薪资
can_read_salary if {
	input.user.roles[_] == "admin"
}

# 最终决策
decision := {
	"allow": count(deny) == 0,
	"denied_fields": denied_fields,
	"reason": concat("; ", deny),
}

# 非 admin 用户 salary 字段被拒绝（适用于 query / mutation / subscription）
denied_fields contains "salary" if {
	input.user.authenticated
	not can_read_salary
}
