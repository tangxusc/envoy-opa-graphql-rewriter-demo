package graphql.authz

default allow = false

default reason = "forbidden"

allow {
  input.field == "publicInfo"
}

allow {
  input.field == "me"
  input.user.authenticated
}

allow_create_post {
  input.user.roles[_] == "editor"
}

allow_create_post {
  input.user.roles[_] == "admin"
}

allow {
  input.field == "createPost"
  input.user.authenticated
  allow_create_post
}

reason = "requires authentication" {
  input.field == "me"
  not input.user.authenticated
} else = "insufficient role" {
  input.field == "createPost"
  input.user.authenticated
  not allow_create_post
} else = "allowed" {
  allow
} else = "forbidden" {
  true
}

decision = {
  "allow": allow,
  "reason": reason,
}
