package concurrent

import "context"

// Stream 将离散数据转化为数据流
// 该函数接收一个上下文和一组值，将这些值发送到返回的通道中
// 当上下文被取消或值发送完毕时，通道会被关闭
func Stream(ctx context.Context, values ...interface{}) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		for _, value := range values {
			select {
			case <-ctx.Done():
				return
			case out <- value:
			}
		}
	}()
	return out
}

// TaskN 只取前n个数据
// 从输入的数据流中获取前n个元素，发送到返回的通道中
// 当获取到n个元素、上下文被取消或输入流关闭时，函数会停止并关闭输出通道
func TaskN(ctx context.Context, valueStream <-chan interface{}, n int) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		for i := 0; i < n; i++ {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if ok == false {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- v:
				}
			}
		}
	}()
	return out
}

// TaskFn 只取满足条件的数据
// 从输入的数据流中过滤出满足给定条件的元素
// 函数会持续从输入流读取数据，只有当元素满足fn函数条件时才会发送到输出通道
// 当上下文被取消或输入流关闭时，函数会停止并关闭输出通道
func TaskFn(ctx context.Context, valueStream <-chan interface{}, fn func(interface{}) bool) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if ok == false {
					return
				}
				if fn(v) {
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				}
			}
		}
	}()
	return out
}

// TaskWhile 只取满足条件的数据,一旦不满足就不再取
func TaskWhile(ctx context.Context, valueStream <-chan interface{}, fn func(interface{}) bool) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if !ok {
					return
				}
				if fn(v) {
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				} else {
					return
				}
			}
		}
	}()
	return out
}

// SkipN 跳过前n个数据
// 从输入的数据流中跳过前n个元素，然后将剩余的元素发送到输出通道
// 先从输入流中读取并丢弃前n个元素，然后将后续所有元素转发到输出通道
// 当上下文被取消或输入流关闭时，函数会停止并关闭输出通道
func SkipN(ctx context.Context, valueStream <-chan interface{}, n int) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		// 跳过前n个元素
		for i := 0; i < n; i++ {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-valueStream:
				if !ok {
					return
				}
			}
		}
		// 转发剩余的元素
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case out <- v:
				}
			}
		}
	}()
	return out
}

// SkipFn 跳过满足条件的数据
// 从输入的数据流中跳过满足给定条件的元素，然后将剩余的元素发送到输出通道
// 对于每个从 valueStream 接收到的元素，如果 fn 函数返回 true，则跳过该元素；
// 如果 fn 函数返回 false，则将该元素发送到输出通道 out
// 当上下文被取消或输入流关闭时，函数会停止并关闭输出通道
func SkipFn(ctx context.Context, valueStream <-chan interface{}, fn func(interface{}) bool) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if !ok {
					return
				}
				// 如果元素不满足跳过条件，则发送到输出通道
				if !fn(v) {
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}
				}
			}
		}
	}()
	return out
}

// SkipWhile 跳过满足条件的数据
// 跳过输入流中连续满足条件的元素，一旦遇到不满足条件的元素，
// 则从该元素开始（包括该元素），将后续所有元素都发送到输出通道
// 这类似于"跳过前缀"的操作，一旦条件不满足，后续所有元素都会被转发
// 当上下文被取消或输入流关闭时，函数会停止并关闭输出通道
func SkipWhile(ctx context.Context, valueStream <-chan interface{}, fn func(interface{}) bool) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		// 第一阶段：跳过连续满足条件的元素
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-valueStream:
				if !ok {
					return
				}
				// 如果元素满足条件，则继续跳过（不发送）
				if fn(v) {
					select {
					case <-ctx.Done():
						return
					default:
					}
				} else {
					// 一旦遇到不满足条件的元素，进入第二阶段
					// 首先将当前元素发送出去
					select {
					case <-ctx.Done():
						return
					case out <- v:
					}

					// 第二阶段：转发所有剩余元素
					for {
						select {
						case <-ctx.Done():
							return
						case v, ok = <-valueStream:
							if !ok {
								return
							}
							select {
							case <-ctx.Done():
								return
							case out <- v:
							}
						}
					}
				}
			}
		}
	}()
	return out
}
