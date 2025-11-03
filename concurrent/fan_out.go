package concurrent

import (
	"sync"
)

// FanOut 扇出模式
func FanOut(in <-chan interface{}, n int, async bool) []chan interface{} {
	wg := sync.WaitGroup{}
	out := make([]chan interface{}, n)
	// 初始化每个 channel
	for i := range out {
		out[i] = make(chan interface{}, 1) // 可以是带缓冲或无缓冲
	}
	go func() {
		defer func() {
			wg.Wait()
			for i := 0; i < n; i++ {
				close(out[i])
			}
		}()

		for v := range in {
			for i := 0; i < n; i++ {
				if async {
					wg.Add(1)
					go func(i int) {
						out[i] <- v
						wg.Done()
					}(i)
				} else {
					out[i] <- v
				}
			}
		}
	}()

	return out
}
