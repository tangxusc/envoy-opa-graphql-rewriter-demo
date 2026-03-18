package graph

import (
	"sync"

	"graphql-server/graph/model"
)

type Resolver struct {
	mu        sync.RWMutex
	employees map[string]*model.Employee
	eventBus  *EventBus
}

func NewResolver() *Resolver {
	r := &Resolver{
		employees: map[string]*model.Employee{
			"emp-1": {ID: "emp-1", Name: "Alice", Salary: 50000},
			"emp-2": {ID: "emp-2", Name: "Bob", Salary: 60000},
			"emp-3": {ID: "emp-3", Name: "Charlie", Salary: 70000},
		},
		eventBus: NewEventBus(),
	}
	return r
}
