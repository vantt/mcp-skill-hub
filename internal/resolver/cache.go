package resolver

import "sync"

type Cache struct {
	mu      sync.RWMutex
	maximum int
	entries map[string]Response
	order   []string
}

func NewCache(maximum int) *Cache {
	if maximum < 1 {
		maximum = 256
	}
	return &Cache{maximum: maximum, entries: make(map[string]Response)}
}

func (cache *Cache) Get(key string) (Response, bool) {
	if cache == nil {
		return Response{}, false
	}
	cache.mu.RLock()
	response, ok := cache.entries[key]
	cache.mu.RUnlock()
	return cloneResponse(response), ok
}

func (cache *Cache) Put(key string, response Response) {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if _, exists := cache.entries[key]; !exists {
		cache.order = append(cache.order, key)
		if len(cache.order) > cache.maximum {
			evicted := cache.order[0]
			delete(cache.entries, evicted)
			cache.order = cache.order[1:]
		}
	}
	cache.entries[key] = cloneResponse(response)
}

func cloneResponse(response Response) Response {
	response.ReasonCodes = append([]string(nil), response.ReasonCodes...)
	response.Supporting = append([]Supporting(nil), response.Supporting...)
	response.Warnings = append([]string(nil), response.Warnings...)
	if response.Primary != nil {
		value := *response.Primary
		response.Primary = &value
	}
	if response.Question != nil {
		value := *response.Question
		value.Choices = append([]string(nil), value.Choices...)
		response.Question = &value
	}
	if response.NoSkill != nil {
		value := *response.NoSkill
		response.NoSkill = &value
	}
	return response
}
