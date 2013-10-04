package hooks

import "sort"

type sorter struct {
	hooks []*Hook
}

func (s *sorter) Len() int {
	return len(s.hooks)
}

func (s *sorter) Swap(i, j int) {
	s.hooks[i], s.hooks[j] = s.hooks[j], s.hooks[i]
}

func (s *sorter) Less(i, j int) bool {
	return s.hooks[i].FileName > s.hooks[j].FileName
}

func Sort(h []*Hook) {
	s := &sorter{h}
	sort.Sort(s)
}
