package gctuner

import (
	"runtime"
	"sync/atomic"
)

type finalizerCallback func()

type finalizer struct {
	ref      *finalizerRef
	callback finalizerCallback // 每次GC都会被调用，因为每次GC都会将ref设置为nil，下次GC会回收为nil引用的数据
	stopped  int32
}

func (f *finalizer) stop() {
	atomic.StoreInt32(&f.stopped, 1)
}

type finalizerRef struct {
	parent *finalizer
}

func finalizerHandler(ref *finalizerRef) {
	// stop calling callback

	if atomic.LoadInt32(&ref.parent.stopped) == 1 {
		return
	}
	ref.parent.callback()
	runtime.SetFinalizer(ref, finalizerHandler)
}

// newFinalizer return a finalizer object and caller should save it to make sure it will not be gc.
// the go runtime promise the callback function should be called every gc time.
func newFinalizer(callback finalizerCallback) *finalizer {
	f := &finalizer{
		callback: callback,
	}
	f.ref = &finalizerRef{
		parent: f,
	}
	runtime.SetFinalizer(f.ref, finalizerHandler)
	f.ref = nil // trigger gc
	return f
}
