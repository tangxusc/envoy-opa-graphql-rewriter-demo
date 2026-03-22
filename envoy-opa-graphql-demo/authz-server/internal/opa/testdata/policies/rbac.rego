package graphqlapi.authz

import rego.v1

can_read_email if {
    hasPrivilege(input.user.privileges, "read:email")
}

denied_fields contains "email" if {
    some field in input.request.fields
    field == "email"
    not can_read_email
}
