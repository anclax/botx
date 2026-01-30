package main

import (
	"fmt"
	"sort"
	"sync"
)

type TodoStore struct {
	mu     sync.Mutex
	nextID int64
	items  map[int64]Todo
}

func NewTodoStore() *TodoStore {
	return &TodoStore{
		nextID: 1,
		items:  make(map[int64]Todo),
	}
}

func (s *TodoStore) List() []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Todo, 0, len(s.items))
	for _, item := range s.items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func (s *TodoStore) Add(title string) Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := Todo{ID: s.nextID, title: title, done: false}
	s.items[item.ID] = item
	s.nextID++
	return item
}

func (s *TodoStore) Get(id int64) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return Todo{}, fmt.Errorf("todo %d not found", id)
	}
	return item, nil
}

func (s *TodoStore) Toggle(id int64) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok {
		return Todo{}, fmt.Errorf("todo %d not found", id)
	}
	item.done = !item.done
	s.items[id] = item
	return item, nil
}

func (s *TodoStore) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return fmt.Errorf("todo %d not found", id)
	}
	delete(s.items, id)
	return nil
}
