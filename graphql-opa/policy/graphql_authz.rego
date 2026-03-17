package graphqlapi.authz

import rego.v1

# 默认拒绝薪资读取。
default can_read_salary := false

# 管理员可读取任意员工薪资。
can_read_salary if {
	input.user.role == "admin"
}

# 普通用户仅可读取本人薪资。
can_read_salary if {
	input.target_id != ""
	input.user.id == input.target_id
}

# 解析并校验输入查询，返回三元数组：
#   下标零：是否通过模式校验
#   下标一：查询语法树（仅在通过校验时可用）
#   下标二：模式语法树（本策略未使用）
parsed := graphql.parse_and_verify(input.input.query, input.input.schema)

# 便于阅读的解析结果别名。
query_valid := parsed[0]

# 拒绝原因一：查询语法无效或与模式不匹配。
reason_set contains "query failed GraphQL schema verification" if {
	not query_valid
}

# 将原因集合转为有序数组，保证输出稳定且可预测。
reasons := sort([reason | reason_set[reason]])

# 统一决策输出，供应用层直接消费。
decision := {
	"allow": query_valid,
	"query_valid": query_valid,
	"reasons": reasons,
}
