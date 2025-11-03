package concurrent

import "context"

// pipeline 最佳实践

// Multiply factor 乘法流水线中需要执行的数据操作
func Multiply(ctx context.Context, valueStream <-chan int, factor int) <-chan int {
	multiplyStream := make(chan int)
	go func() {
		defer close(multiplyStream)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if ok == false {
					return
				}
				multiplyStream <- v * factor
			}
		}
	}()
	return multiplyStream
}

func Add(ctx context.Context, valueStream <-chan int, delta int) <-chan int {
	addStream := make(chan int)
	go func() {
		defer close(addStream)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if ok == false {
					return
				}
				addStream <- v + delta
			}
		}
	}()
	return addStream
}
