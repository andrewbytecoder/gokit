package channel

// Or 复合channel 当任意一个channel关闭时，返回
func Or(channels ...<-chan interface{}) <-chan interface{} {
	// 根据需要可以作为散入和散出的作用，多个chan进来合并成一个channel
	switch len(channels) {
	case 0:
		return nil
	case 1:
		return channels[0]
	}

	orDone := make(chan interface{})
	go func() {
		defer close(orDone)
		switch len(channels) {
		case 2:
			select {
			case <-channels[0]:
			case <-channels[1]:
			}
		default:
			select {
			case <-channels[0]:
			case <-channels[1]:
			case <-channels[2]:
			case <-Or(append(channels[3:], orDone)...):
			}
		}
	}()
	return orDone
}

// pipeline 最佳实践

// Generator 将离散数据转化为数据流
func Generator(done <-chan interface{}, values ...int) <-chan int {
	valueStream := make(chan int)
	go func() {
		defer close(valueStream)
		for _, v := range values {
			select {
			case <-done:
				return
			case valueStream <- v:
			}
		}
	}()
	return valueStream
}

// Multiply factor 乘法流水线中需要执行的数据操作
func Multiply(done <-chan interface{}, valueStream <-chan int, factor int) <-chan int {
	multiplyStream := make(chan int)
	go func() {
		defer close(multiplyStream)
		for {
			select {
			case <-done:
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

func Add(done <-chan interface{}, valueStream <-chan int, delta int) <-chan int {
	addStream := make(chan int)
	go func() {
		defer close(addStream)
		for {
			select {
			case <-done:
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
