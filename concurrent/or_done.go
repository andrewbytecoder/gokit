package concurrent

import "reflect"

// OrDone 任意chan关闭时，返回
// 相比于Or来说减少性能消耗，性能高
func OrDone(channels ...<-chan interface{}) <-chan interface{} {
	switch len(channels) {
	case 0:
		// 返回已经关闭的channel 通知各个接受者关闭
		c := make(chan interface{})
		close(c)
		return c
	case 1:
		return channels[0]
	}
	orDone := make(chan interface{}, 1)
	go func() {
		defer close(orDone)
		var cases []reflect.SelectCase
		for _, channel := range channels {
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(channel),
			})
		}
		// 选择一个可用的，chan关闭的时候后会变成妃阻塞的情况借助这种形式进行判断是否有chan已经关闭
		reflect.Select(cases)
	}()
	return orDone
}

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
