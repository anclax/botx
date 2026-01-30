package common

import (
	"context"
	"net/url"
	"strings"
	"sync"

	"github.com/anclax/botx/pkg/core/bot"
	"github.com/pkg/errors"
)

type addressEntry struct {
	ID      int64
	Address string
	Name    string
}

type AddressStore struct {
	mu     sync.RWMutex
	nextID int64
	items  []addressEntry
}

func NewAddressStore() *AddressStore {
	return &AddressStore{nextID: 1}
}

func (s *AddressStore) list() []addressEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]addressEntry, len(s.items))
	copy(items, s.items)
	return items
}

func (s *AddressStore) get(id int64) (addressEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.items {
		if item.ID == id {
			return item, true
		}
	}
	return addressEntry{}, false
}

func (s *AddressStore) add(address string, name string) addressEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := addressEntry{ID: s.nextID, Address: address, Name: name}
	s.nextID++
	s.items = append(s.items, entry)
	return entry
}

func (s *AddressStore) delete(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.items {
		if item.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return true
		}
	}
	return false
}

func (s *AddressStore) update(id int64, field string, value string) (addressEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.items {
		if item.ID != id {
			continue
		}
		switch field {
		case "name":
			item.Name = value
		case "address":
			item.Address = value
		default:
			return addressEntry{}, false
		}
		s.items[i] = item
		return item, true
	}
	return addressEntry{}, false
}

type SampleStateProvider struct {
	store *AddressStore
}

func NewSampleStateProvider(store *AddressStore) *SampleStateProvider {
	return &SampleStateProvider{store: store}
}

func (s *SampleStateProvider) ProvideRootState(_ context.Context, _ int64, _ *ParametersPageRoot) (*StatePageRoot, error) {
	return &StatePageRoot{}, nil
}

func (s *SampleStateProvider) ProvideAddressState(_ context.Context, _ int64, parameters *ParametersPageAddress) (*StatePageAddress, error) {
	items := s.store.list()
	pageSize := parameters.GetRow() * parameters.GetColumn()
	if pageSize <= 0 {
		pageSize = len(items)
		if pageSize == 0 {
			pageSize = 1
		}
	}
	start := parameters.GetPage() * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}

	page := make([]Address, 0, end-start)
	for _, item := range items[start:end] {
		page = append(page, *NewAddress(item.ID, item.Address, item.Name))
	}
	return NewStatePageAddress(page, len(items)), nil
}

func (s *SampleStateProvider) ProvideAddressIDState(_ context.Context, _ int64, parameters *ParametersPageAddressID) (*StatePageAddressID, error) {
	item, ok := s.store.get(parameters.GetID())
	if !ok {
		return nil, errors.Wrapf(bot.ErrNotFound, "address %d not found", parameters.GetID())
	}
	return NewStatePageAddressID(*NewAddress(item.ID, item.Address, item.Name)), nil
}

func (s *SampleStateProvider) ProvideAddressAddState(_ context.Context, _ int64, form *FormAddressAdd, _ *ParametersPageAddressAdd) (*StatePageAddressAdd, error) {
	address, name := parseAddressInput(form.GetAddress())
	if address == "" {
		return NewStatePageAddressAdd(false, "address is required"), nil
	}
	s.store.add(address, name)
	return NewStatePageAddressAdd(true, ""), nil
}

func (s *SampleStateProvider) ProvideAddressDeleteState(_ context.Context, _ int64, parameters *ParametersPageAddressDelete) (*StatePageAddressDelete, error) {
	if !s.store.delete(parameters.GetID()) {
		return NewStatePageAddressDelete(false, "address not found"), nil
	}
	return NewStatePageAddressDelete(true, ""), nil
}

func (s *SampleStateProvider) ProvideAddressEditState(_ context.Context, _ int64, form *FormAddressEdit, parameters *ParametersPageAddressEdit) (*StatePageAddressEdit, error) {
	value := strings.TrimSpace(form.GetValue())
	if value == "" {
		return NewStatePageAddressEdit(false, "value is required"), nil
	}
	if _, ok := s.store.update(parameters.GetID(), parameters.GetField(), value); !ok {
		return NewStatePageAddressEdit(false, "address not found"), nil
	}
	return NewStatePageAddressEdit(true, ""), nil
}

type SampleFormValidator struct{}

func (v *SampleFormValidator) ValidateFormAddressAdd(_ context.Context, _ int64, _ *url.URL, input string) (*bot.ValidateResult, error) {
	if strings.TrimSpace(input) == "" {
		return &bot.ValidateResult{
			Valid:        false,
			ErrorMessage: "address is required",
		}, nil
	}
	return &bot.ValidateResult{Valid: true}, nil
}

func parseAddressInput(input string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return "", ""
	}
	address := parts[0]
	name := ""
	if len(parts) > 1 {
		name = strings.Join(parts[1:], " ")
	}
	return address, name
}
