package graph

import (
	"sync"
	"testing"
	"time"

	"graphql-server/graph/model"
)

func TestEventBus_SubscribePublish(t *testing.T) {
	t.Parallel()
	eb := NewEventBus()

	id, ch := eb.Subscribe()
	defer eb.Unsubscribe(id)

	emp := &model.Employee{ID: "emp-1", Name: "Alice", Salary: 50000}
	eb.Publish(emp)

	select {
	case got := <-ch:
		if got.ID != emp.ID || got.Name != emp.Name {
			t.Errorf("got %+v, want %+v", got, emp)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for published event")
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	t.Parallel()
	eb := NewEventBus()

	id1, ch1 := eb.Subscribe()
	id2, ch2 := eb.Subscribe()
	defer eb.Unsubscribe(id1)
	defer eb.Unsubscribe(id2)

	emp := &model.Employee{ID: "emp-2", Name: "Bob", Salary: 60000}
	eb.Publish(emp)

	for i, ch := range []<-chan *model.Employee{ch1, ch2} {
		select {
		case got := <-ch:
			if got.ID != emp.ID {
				t.Errorf("subscriber %d: got ID %q, want %q", i, got.ID, emp.ID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestEventBus_UnsubscribeStopsReceiving(t *testing.T) {
	t.Parallel()
	eb := NewEventBus()

	id, ch := eb.Subscribe()
	eb.Unsubscribe(id)

	// channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}

	// Publishing after unsubscribe should not panic
	emp := &model.Employee{ID: "emp-1", Name: "Alice", Salary: 50000}
	eb.Publish(emp) // should not panic
}

func TestEventBus_NonBlockingPublish(t *testing.T) {
	t.Parallel()
	eb := NewEventBus()

	id, _ := eb.Subscribe() // buffer size 1, don't read from channel
	defer eb.Unsubscribe(id)

	// Fill the buffer
	eb.Publish(&model.Employee{ID: "emp-1", Name: "Alice", Salary: 50000})
	// This should not block even though buffer is full
	eb.Publish(&model.Employee{ID: "emp-2", Name: "Bob", Salary: 60000})
}

func TestEventBus_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	eb := NewEventBus()

	var wg sync.WaitGroup
	// Concurrent subscribers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, ch := eb.Subscribe()
			// Read one event then unsubscribe
			go func() {
				<-ch // might be closed or receive event
			}()
			time.Sleep(10 * time.Millisecond)
			eb.Unsubscribe(id)
		}()
	}

	// Concurrent publishers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eb.Publish(&model.Employee{ID: "emp-1", Name: "Alice", Salary: 50000})
		}()
	}

	wg.Wait()
}
