package graphqlapi.authz

import rego.v1

can_read_department if {
    hasPrivilege(input.user.privileges, "read:department")
}

denied_fields contains "department" if {
    some field in input.request.fields
    field == "department"
    not can_read_department
}
