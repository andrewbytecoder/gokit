package concurrent

import (
	"sync"
)

type WaitGroup interface {
	Add(int)
	Wait()
	Done()
	Do()
}

type OrderlyTask struct {
	sync.WaitGroup
}
