package channel

// 使用chan控制一个协程的退出，虽然也能实现，推荐使用context

func OrderData() {
	channel := make(chan struct{})
	go func() {

		<-channel
	}()

	// 主函数退出时，关闭channel，或者向channel中发送一个struct{}

	//close(channel)
	channel <- struct{}{}
}
