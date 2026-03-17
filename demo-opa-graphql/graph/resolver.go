package graph

import (
	"fmt"
	"sync"

	"demo-opa-graphql/internal/security"
)

type UserRecord struct {
	ID    string
	Name  string
	Roles []string
}

type Resolver struct {
	Authorizer *security.Authorizer

	mu      sync.Mutex
	users   map[string]UserRecord
	postSeq int
}

func NewResolver(authorizer *security.Authorizer) *Resolver {
	return &Resolver{
		Authorizer: authorizer,
		users: map[string]UserRecord{
			"user-1": {
				ID:    "user-1",
				Name:  "Alice",
				Roles: []string{"user"},
			},
			"admin-1": {
				ID:    "admin-1",
				Name:  "Bob",
				Roles: []string{"admin"},
			},
		},
		postSeq: 1000,
	}
}

func (r *Resolver) nextPostID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.postSeq++
	return fmt.Sprintf("post-%d", r.postSeq)
}

func (r *Resolver) lookupUser(subject string, fallbackRoles []string) UserRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.users[subject]
	if ok {
		record.Roles = unionRoles(record.Roles, fallbackRoles)
		return record
	}

	record = UserRecord{
		ID:    subject,
		Name:  subject,
		Roles: unionRoles(nil, fallbackRoles),
	}
	r.users[subject] = record
	return record
}
