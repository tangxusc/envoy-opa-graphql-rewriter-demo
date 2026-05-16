package graphqlapi.authz

import rego.v1

# 默认值
default allow := false
default can_read_salary := false

# 未认证用户直接拒绝
deny contains "unauthenticated request" if {
	not input.user.authenticated
}

# 使用 hasPrivilege 检查薪资读取权限
can_read_salary if {
	hasPrivilege(input.user.privileges, "read:salary")
}

# 最终决策
decision := {
	"allow": count(deny) == 0,
	"denied_fields": denied_fields,
	"reason": concat("; ", deny),
}

# 无 read:salary 权限的用户，salary 字段在 employeeByID 路径下被拒绝
denied_fields contains "employeeByID.salary" if {
	input.user.authenticated
	not can_read_salary
}
