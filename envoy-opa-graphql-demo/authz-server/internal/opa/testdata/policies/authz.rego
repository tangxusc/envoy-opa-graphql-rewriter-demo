package graphqlapi.authz

import rego.v1

default decision := {
    "allow": false,
    "denied_fields": [],
    "reason": "denied by default",
}

decision := d if {
    input.user.authenticated
    d := {
        "allow": true,
        "denied_fields": denied_fields,
        "reason": "",
    }
}

denied_fields contains "salary" if {
    some field in input.request.fields
    field == "salary"
    not has_salary_privilege
}

default has_salary_privilege := false

has_salary_privilege if {
    hasPrivilege(input.user.privileges, "read:salary")
}
