package graph

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"todo-server/graph/model"
)

type Resolver struct {
	mu              sync.RWMutex
	todos           map[string]*model.Todo
	todosByEmployee map[string]map[string]struct{}
	sequence        atomic.Uint64
	publisher       EventPublisher
}

func NewResolver(publisher EventPublisher) *Resolver {
	if publisher == nil {
		publisher = NoopPublisher{}
	}

	r := &Resolver{
		todos: map[string]*model.Todo{
			"todo-1": {
				ID:         "todo-1",
				EmployeeID: "emp-1",
				Content:    "Complete onboarding docs",
				UpdatedAt:  nowRFC3339(),
				Deleted:    false,
			},
			"todo-2": {
				ID:         "todo-2",
				EmployeeID: "emp-2",
				Content:    "Prepare quarterly report",
				UpdatedAt:  nowRFC3339(),
				Deleted:    false,
			},
		},
		todosByEmployee: make(map[string]map[string]struct{}),
		publisher:       publisher,
	}
	r.sequence.Store(2)
	for id, todo := range r.todos {
		if _, ok := r.todosByEmployee[todo.EmployeeID]; !ok {
			r.todosByEmployee[todo.EmployeeID] = make(map[string]struct{})
		}
		r.todosByEmployee[todo.EmployeeID][id] = struct{}{}
	}
	return r
}

func (r *Resolver) Close() error {
	if r.publisher == nil {
		return nil
	}
	return r.publisher.Close()
}

func (r *Resolver) nextTodoID() string {
	seq := r.sequence.Add(1)
	return fmt.Sprintf("todo-%d", seq)
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func cloneTodo(todo *model.Todo) *model.Todo {
	if todo == nil {
		return nil
	}
	copied := *todo
	return &copied
}

func (r *Resolver) upsertEmployeeIndexNoLock(employeeID, todoID string) {
	index, ok := r.todosByEmployee[employeeID]
	if !ok {
		index = make(map[string]struct{})
		r.todosByEmployee[employeeID] = index
	}
	index[todoID] = struct{}{}
}

func (r *Resolver) removeEmployeeIndexNoLock(employeeID, todoID string) {
	index, ok := r.todosByEmployee[employeeID]
	if !ok {
		return
	}
	delete(index, todoID)
	if len(index) == 0 {
		delete(r.todosByEmployee, employeeID)
	}
}

func (r *Resolver) listTodosByEmployeeNoLock(employeeID string) []*model.Todo {
	ids := r.todosByEmployee[employeeID]
	if len(ids) == 0 {
		return []*model.Todo{}
	}

	items := make([]*model.Todo, 0, len(ids))
	for todoID := range ids {
		todo, ok := r.todos[todoID]
		if !ok || todo.Deleted {
			continue
		}
		items = append(items, cloneTodo(todo))
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items
}

func (r *Resolver) publishTodoEvent(ctx context.Context, eventType string, todo *model.Todo) error {
	event := TodoEvent{
		EventID:    fmt.Sprintf("%s-%d", eventType, time.Now().UTC().UnixNano()),
		EventType:  eventType,
		OccurredAt: nowRFC3339(),
		ID:         todo.EmployeeID,
		TodoID:     todo.ID,
		EmployeeID: todo.EmployeeID,
		Content:    todo.Content,
		Deleted:    todo.Deleted,
	}
	return r.publisher.PublishTodoEvent(ctx, event)
}
