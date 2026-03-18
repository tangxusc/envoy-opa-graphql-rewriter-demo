package graph

import (
	"sync"
	"sync/atomic"

	"graphql-server/graph/model"
)

// EventBus 是基于 Go channel 的进程内 pub/sub 系统。
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan *model.Employee
	nextID      atomic.Uint64
}

// NewEventBus 创建新的 EventBus 实例。
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[uint64]chan *model.Employee),
	}
}

// Subscribe 注册一个新的订阅者，返回订阅 ID 和接收 channel。
func (eb *EventBus) Subscribe() (uint64, <-chan *model.Employee) {
	id := eb.nextID.Add(1)
	ch := make(chan *model.Employee, 1) // buffered to avoid blocking slow subscribers

	eb.mu.Lock()
	eb.subscribers[id] = ch
	eb.mu.Unlock()

	return id, ch
}

// Unsubscribe 取消订阅并关闭对应的 channel。
func (eb *EventBus) Unsubscribe(id uint64) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if ch, ok := eb.subscribers[id]; ok {
		delete(eb.subscribers, id)
		close(ch)
	}
}

// Publish 将员工更新事件非阻塞地广播给所有订阅者。
// 如果某个订阅者的 channel 已满，该事件将被丢弃（不阻塞其他订阅者）。
func (eb *EventBus) Publish(emp *model.Employee) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for _, ch := range eb.subscribers {
		select {
		case ch <- emp:
		default:
			// 慢订阅者 — 丢弃事件，不阻塞
		}
	}
}
