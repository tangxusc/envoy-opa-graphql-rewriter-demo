package graphqlapi.authz

import rego.v1

default decision := {
    "allow": false,
    "denied_fields": [],
    "reason": "denied by default",
}

# Authenticated users are allowed.
decision := d if {
    input.user.authenticated
    d := {
        "allow": true,
        "denied_fields": denied_fields,
        "reason": "",
    }
}

# Unauthenticated users are denied.
decision := d if {
    not input.user.authenticated
    d := {
        "allow": false,
        "denied_fields": [],
        "reason": "unauthenticated",
    }
}

# Salary field is denied for non-admin roles.
denied_fields contains "salary" if {
    some field in input.request.fields
    field == "salary"
    not is_admin
}

default is_admin := false

is_admin if {
    some role in input.user.roles
    role == "admin"
}
