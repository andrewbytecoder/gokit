package deplyqueue

import (
	"container/heap"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/utils/clock"
	clocktesting "k8s.io/utils/clock/testing"
)

// =========================== Test Helpers ===========================

// newDelayingType creates a DelayingType with a fake clock and starts its waitingLoop.
func newDelayingType[T comparable](q TypedInterface[T], c clock.WithTicker) *DelayingType[T] {
	d := &DelayingType[T]{
		TypedInterface:  q,
		clock:           c,
		stopCh:          make(chan struct{}),
		waitingForAddCh: make(chan *waitFor[T], 1000),
		heartbeat:       c.NewTicker(maxWait),
	}
	return d
}

// mockTypedInterface is a minimal TypedInterface for testing DelayingType.
type mockTypedInterface[T comparable] struct {
	mu       sync.Mutex
	items    []T
	added    []T // record adds for assertion
	shutDown bool
	addCh    chan T // for notifying Get()
}

func newMockTypedInterface[T comparable]() *mockTypedInterface[T] {
	return &mockTypedInterface[T]{
		addCh: make(chan T, 1000),
	}
}

func (m *mockTypedInterface[T]) Add(item T) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, item)
	m.added = append(m.added, item)
	select {
	case m.addCh <- item:
	default:
	}
}

func (m *mockTypedInterface[T]) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

func (m *mockTypedInterface[T]) Get() (item T, shutdown bool) {
	select {
	case item = <-m.addCh:
		return item, m.ShuttingDown()
	}
}

func (m *mockTypedInterface[T]) Done(item T) {}

func (m *mockTypedInterface[T]) ShutDown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutDown = true
}

func (m *mockTypedInterface[T]) ShutDownWithDrain() {
	m.ShutDown()
}

func (m *mockTypedInterface[T]) ShuttingDown() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.shutDown
}

// addedItems returns a copy of all items added (for assertion).
func (m *mockTypedInterface[T]) addedItems() []T {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]T(nil), m.added...)
}

// =========================== waitForPriorityQueue Tests ===========================

func TestWaitForPriorityQueue_Len(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	if pq.Len() != 0 {
		t.Fatalf("expected len 0, got %d", pq.Len())
	}
	heap.Push(pq, &waitFor[int]{data: 1, readyAt: time.Unix(10, 0)})
	if pq.Len() != 1 {
		t.Fatalf("expected len 1, got %d", pq.Len())
	}
}

func TestWaitForPriorityQueue_PushPop(t *testing.T) {
	pq := &waitForPriorityQueue[string]{}
	heap.Init(pq)

	heap.Push(pq, &waitFor[string]{data: "c", readyAt: time.Unix(30, 0)})
	heap.Push(pq, &waitFor[string]{data: "a", readyAt: time.Unix(10, 0)})
	heap.Push(pq, &waitFor[string]{data: "b", readyAt: time.Unix(20, 0)})

	// Should pop in time order: a, b, c
	item := heap.Pop(pq).(*waitFor[string])
	if item.data != "a" {
		t.Fatalf("expected 'a', got '%s'", item.data)
	}
	item = heap.Pop(pq).(*waitFor[string])
	if item.data != "b" {
		t.Fatalf("expected 'b', got '%s'", item.data)
	}
	item = heap.Pop(pq).(*waitFor[string])
	if item.data != "c" {
		t.Fatalf("expected 'c', got '%s'", item.data)
	}
	if pq.Len() != 0 {
		t.Fatal("expected empty queue")
	}
}

func TestWaitForPriorityQueue_Peek(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)

	heap.Push(pq, &waitFor[int]{data: 2, readyAt: time.Unix(20, 0)})
	heap.Push(pq, &waitFor[int]{data: 1, readyAt: time.Unix(10, 0)})

	item := pq.Peek().(*waitFor[int])
	if item.data != 1 {
		t.Fatalf("expected data 1, got %d", item.data)
	}
	if pq.Len() != 2 {
		t.Fatal("Peek should not remove items")
	}
}

func TestWaitForPriorityQueue_Swap(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)

	heap.Push(pq, &waitFor[int]{data: 1, readyAt: time.Unix(10, 0)})
	heap.Push(pq, &waitFor[int]{data: 2, readyAt: time.Unix(20, 0)})

	pq.Swap(0, 1)

	if (*pq)[0].data != 2 || (*pq)[1].data != 1 {
		t.Fatal("Swap did not swap correctly")
	}
	if (*pq)[0].index != 0 || (*pq)[1].index != 1 {
		t.Fatal("Swap did not update indices")
	}
}

func TestWaitForPriorityQueue_SameReadyAt(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)

	sameTime := time.Unix(100, 0)
	heap.Push(pq, &waitFor[int]{data: 1, readyAt: sameTime})
	heap.Push(pq, &waitFor[int]{data: 2, readyAt: sameTime})
	heap.Push(pq, &waitFor[int]{data: 3, readyAt: sameTime})

	if pq.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", pq.Len())
	}

	// All should pop without error
	heap.Pop(pq)
	heap.Pop(pq)
	heap.Pop(pq)

	if pq.Len() != 0 {
		t.Fatal("expected empty queue after pops")
	}
}

// =========================== insert Tests ===========================

func TestInsert_NewEntry(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	known := map[int]*waitFor[int]{}

	entry := &waitFor[int]{data: 42, readyAt: time.Unix(50, 0)}
	insert(pq, known, entry)

	if pq.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", pq.Len())
	}
	if known[42] != entry {
		t.Fatal("entry not tracked in knownEntries")
	}
}

func TestInsert_UpdateEarlier(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	known := map[int]*waitFor[int]{}

	entry := &waitFor[int]{data: 42, readyAt: time.Unix(50, 0)}
	insert(pq, known, entry)

	// Update with earlier readyAt
	entry2 := &waitFor[int]{data: 42, readyAt: time.Unix(30, 0)}
	insert(pq, known, entry2)

	if pq.Len() != 1 {
		t.Fatalf("expected still 1 item, got %d", pq.Len())
	}
	if known[42].readyAt != time.Unix(30, 0) {
		t.Fatal("readyAt was not updated to earlier time")
	}
	// Peek should return the item with updated time
	peeked := pq.Peek().(*waitFor[int])
	if !peeked.readyAt.Equal(time.Unix(30, 0)) {
		t.Fatalf("expected readyAt 30, got %v", peeked.readyAt)
	}
}

func TestInsert_UpdateLaterIgnored(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	known := map[int]*waitFor[int]{}

	entry := &waitFor[int]{data: 42, readyAt: time.Unix(30, 0)}
	insert(pq, known, entry)

	// Try to update with later readyAt — should be ignored
	entry2 := &waitFor[int]{data: 42, readyAt: time.Unix(50, 0)}
	insert(pq, known, entry2)

	if !known[42].readyAt.Equal(time.Unix(30, 0)) {
		t.Fatal("readyAt should NOT be updated to later time")
	}
}

func TestInsert_MultipleEntries(t *testing.T) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	known := map[int]*waitFor[int]{}

	insert(pq, known, &waitFor[int]{data: 1, readyAt: time.Unix(30, 0)})
	insert(pq, known, &waitFor[int]{data: 2, readyAt: time.Unix(10, 0)})
	insert(pq, known, &waitFor[int]{data: 3, readyAt: time.Unix(20, 0)})

	if pq.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", pq.Len())
	}

	// Should come out in time order: 2, 3, 1
	first := heap.Pop(pq).(*waitFor[int])
	if first.data != 2 {
		t.Fatalf("expected 2, got %d", first.data)
	}
	second := heap.Pop(pq).(*waitFor[int])
	if second.data != 3 {
		t.Fatalf("expected 3, got %d", second.data)
	}
	third := heap.Pop(pq).(*waitFor[int])
	if third.data != 1 {
		t.Fatalf("expected 1, got %d", third.data)
	}
}

// =========================== DelayingType Tests ===========================

// startWaitingLoop starts the waitingLoop in a background goroutine.
func startWaitingLoop[T comparable](d *DelayingType[T]) {
	go d.waitingLoop()
}

func TestDelayingType_AddAfter_Immediate(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	// Should add immediately with zero delay
	d.AddAfter(1, 0)
	// Give some time for the goroutine
	time.Sleep(10 * time.Millisecond)
	if mockQ.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", mockQ.Len())
	}

	// Should add immediately with negative delay
	d.AddAfter(2, -time.Second)
	time.Sleep(10 * time.Millisecond)
	if mockQ.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", mockQ.Len())
	}
}

func TestDelayingType_AddAfter_Delayed(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	// Add with a delay of 5 seconds
	d.AddAfter(99, 5*time.Second)
	time.Sleep(20 * time.Millisecond) // let the goroutine pick it up

	if mockQ.Len() != 0 {
		t.Fatal("item should not be added yet")
	}

	// Step 4 seconds — still not ready
	fakeClock.Step(4 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 0 {
		t.Fatal("item should still not be added after 4s")
	}

	// Step remaining 1 second — should fire
	fakeClock.Step(1 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", mockQ.Len())
	}
}

func TestDelayingType_AddAfter_MultipleDelays(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	// Add items with different delays
	d.AddAfter(10, 10*time.Second)
	d.AddAfter(5, 5*time.Second)
	d.AddAfter(7, 7*time.Second)
	time.Sleep(20 * time.Millisecond)

	// Step 5s: item 5 should be added
	fakeClock.Step(5 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 1 {
		t.Fatalf("expected 1 item after 5s, got %d", mockQ.Len())
	}

	// Step 2s more (total 7s): item 7 should be added
	fakeClock.Step(2 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 2 {
		t.Fatalf("expected 2 items after 7s, got %d", mockQ.Len())
	}

	// Step 3s more (total 10s): item 10 should be added
	fakeClock.Step(3 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 3 {
		t.Fatalf("expected 3 items after 10s, got %d", mockQ.Len())
	}

	items := mockQ.addedItems()
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// Items should be added in time order: 5, 7, 10
	if items[0] != 5 {
		t.Fatalf("expected first item 5, got %d", items[0])
	}
	if items[1] != 7 {
		t.Fatalf("expected second item 7, got %d", items[1])
	}
	if items[2] != 10 {
		t.Fatalf("expected third item 10, got %d", items[2])
	}
}

func TestDelayingType_AddAfter_ShuttingDown(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)

	// Shutdown first
	d.ShutDown()
	time.Sleep(10 * time.Millisecond)

	// These should be ignored
	d.AddAfter(1, 0)
	d.AddAfter(2, 5*time.Second)
	time.Sleep(20 * time.Millisecond)

	if mockQ.Len() != 0 {
		t.Fatalf("expected 0 items after shutdown, got %d", mockQ.Len())
	}
}

func TestDelayingType_ShutDown(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)

	if d.ShuttingDown() {
		t.Fatal("should not be shutting down")
	}

	d.ShutDown()
	time.Sleep(20 * time.Millisecond)

	if !d.ShuttingDown() {
		t.Fatal("should be shutting down after ShutDown")
	}

	// Calling ShutDown again should be safe (sync.Once)
	d.ShutDown()
}

func TestDelayingType_AddAfter_UpdateEarlierDelay(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[string]()
	d := newDelayingType[string](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	// First add with a 10s delay
	d.AddAfter("item", 10*time.Second)
	time.Sleep(20 * time.Millisecond)

	// Then add the same item with a shorter 3s delay (should update)
	d.AddAfter("item", 3*time.Second)
	time.Sleep(20 * time.Millisecond)

	// Step 3s — item should fire at the earlier time
	fakeClock.Step(3 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 1 {
		t.Fatalf("expected 1 item after 3s, got %d", mockQ.Len())
	}
}

func TestDelayingType_AddAfter_ImmediateAlreadyExpired(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(100, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	// Add with a short delay, then step clock past it so it's expired
	d.AddAfter(42, 1*time.Second)
	time.Sleep(20 * time.Millisecond)

	if mockQ.Len() != 0 {
		t.Fatal("item should not be added before clock step")
	}

	// Now step the clock past readyAt — the item should be picked up as expired
	fakeClock.Step(2 * time.Second)
	time.Sleep(20 * time.Millisecond)

	if mockQ.Len() != 1 {
		t.Fatalf("expected item added after clock step past readyAt, got %d", mockQ.Len())
	}
}

func TestDelayingType_AddAfter_AddDuplicateThenEarlier(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	// Add with 10s delay
	d.AddAfter(99, 10*time.Second)
	time.Sleep(20 * time.Millisecond)

	// Step to 5s, then add same item with 5s delay (earlier than original readyAt)
	fakeClock.Step(5 * time.Second)
	d.AddAfter(99, 5*time.Second) // readyAt = 10s from now, but original was at 10s absolute
	time.Sleep(20 * time.Millisecond)

	// Step to 10s absolute, item should fire
	fakeClock.Step(5 * time.Second)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", mockQ.Len())
	}
}

func TestDelayingType_AddAfter_ShuttingDownRace(t *testing.T) {
	// Test that AddAfter is safe when called concurrently with ShutDown
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				d.AddAfter(val, time.Millisecond)
			}
		}(i)
	}

	// Shutdown while items are being added
	time.Sleep(5 * time.Millisecond)
	d.ShutDown()
	wg.Wait()
}

// =========================== Example Tests ===========================

func ExampleDelayingType_AddAfter() {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)

	// Add items with different delays
	d.AddAfter(1, 0)
	d.AddAfter(2, 5*time.Second)

	time.Sleep(10 * time.Millisecond)

	fmt.Printf("After 0s: len=%d\n", mockQ.Len())

	fakeClock.Step(5 * time.Second)
	time.Sleep(10 * time.Millisecond)

	fmt.Printf("After 5s: len=%d\n", mockQ.Len())

	d.ShutDown()

	// Output:
	// After 0s: len=1
	// After 5s: len=2
}

func ExampleDelayingType_ShutDown() {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)

	fmt.Printf("Before: shuttingDown=%v\n", d.ShuttingDown())

	d.ShutDown()
	time.Sleep(10 * time.Millisecond)

	fmt.Printf("After: shuttingDown=%v\n", d.ShuttingDown())

	// Output:
	// Before: shuttingDown=false
	// After: shuttingDown=true
}

// =========================== Benchmark Tests ===========================

// BenchmarkWaitForPriorityQueue_Push benchmarks pushing items onto the priority queue.
func BenchmarkWaitForPriorityQueue_Push(b *testing.B) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	now := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		heap.Push(pq, &waitFor[int]{data: i, readyAt: now.Add(time.Duration(i) * time.Second)})
	}
}

// BenchmarkWaitForPriorityQueue_Pop benchmarks popping items from the priority queue.
func BenchmarkWaitForPriorityQueue_Pop(b *testing.B) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	now := time.Now()
	for i := 0; i < b.N; i++ {
		heap.Push(pq, &waitFor[int]{data: i, readyAt: now.Add(time.Duration(i) * time.Second)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		heap.Pop(pq)
	}
}

// BenchmarkWaitForPriorityQueue_PushPop benchmarks alternating push/pop.
func BenchmarkWaitForPriorityQueue_PushPop(b *testing.B) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	now := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		heap.Push(pq, &waitFor[int]{data: i, readyAt: now.Add(time.Duration(i) * time.Millisecond)})
		heap.Pop(pq)
	}
}

// BenchmarkInsert_NewEntry benchmarks inserting new entries.
func BenchmarkInsert_NewEntry(b *testing.B) {
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	known := map[int]*waitFor[int]{}
	now := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		insert(pq, known, &waitFor[int]{data: i, readyAt: now.Add(time.Duration(i) * time.Second)})
	}
}

// BenchmarkInsert_UpdateExisting benchmarks updating existing entries.
func BenchmarkInsert_UpdateExisting(b *testing.B) {
	const prePop = 10000
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	known := map[int]*waitFor[int]{}
	now := time.Now()

	// Pre-populate with a fixed number of entries
	for i := 0; i < prePop; i++ {
		insert(pq, known, &waitFor[int]{data: i, readyAt: now.Add(time.Duration(i+10) * time.Second)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		insert(pq, known, &waitFor[int]{data: i % prePop, readyAt: now.Add(time.Duration(i) * time.Second)})
	}
}

// BenchmarkInsert_Mixed benchmarks a mix of new and updating inserts.
func BenchmarkInsert_Mixed(b *testing.B) {
	const prePop = 10000
	pq := &waitForPriorityQueue[int]{}
	heap.Init(pq)
	known := map[int]*waitFor[int]{}
	now := time.Now()

	// Pre-populate half
	for i := 0; i < prePop/2; i++ {
		insert(pq, known, &waitFor[int]{data: i, readyAt: now.Add(time.Duration(i+10) * time.Second)})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		insert(pq, known, &waitFor[int]{data: i + prePop, readyAt: now.Add(time.Duration(i) * time.Second)})
	}
}

// BenchmarkDelayingType_AddAfter_Immediate benchmarks immediate AddAfter.
func BenchmarkDelayingType_AddAfter_Immediate(b *testing.B) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.AddAfter(i, 0)
	}
}

// BenchmarkDelayingType_AddAfter_Delayed benchmarks delayed AddAfter with clock stepping to drain.
func BenchmarkDelayingType_AddAfter_Delayed(b *testing.B) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.AddAfter(i, time.Second)
		fakeClock.Step(2 * time.Second)
	}
}

// BenchmarkDelayingType_ShutDown benchmarks ShutDown.
func BenchmarkDelayingType_ShutDown(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
		mockQ := newMockTypedInterface[int]()
		d := newDelayingType[int](mockQ, fakeClock)
		startWaitingLoop(d)
		b.StartTimer()

		d.ShutDown()
	}
}

// BenchmarkContainerHeap_Push benchmarks standard container/heap Push.
func BenchmarkContainerHeap_Push(b *testing.B) {
	pq := make(heapImpl2, 0)
	now := time.Now()

	heap.Init(&pq)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		heap.Push(&pq, &heapItem2{v: i, time: now.Add(time.Duration(i) * time.Second)})
	}
}

// heapImpl2 adapts []*item2 to heap.Interface for comparison benchmarking.
type heapImpl2 []*heapItem2

type heapItem2 struct {
	v    int
	time time.Time
	idx  int
}

func (h heapImpl2) Len() int { return len(h) }
func (h heapImpl2) Less(i, j int) bool {
	return h[i].time.Before(h[j].time)
}
func (h heapImpl2) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].idx = i
	h[j].idx = j
}
func (h *heapImpl2) Push(x any) {
	n := len(*h)
	it := x.(*heapItem2)
	it.idx = n
	*h = append(*h, it)
}
func (h *heapImpl2) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	it.idx = -1
	*h = old[0 : n-1]
	return it
}

// =========================== Integration Test: full waitingLoop cycle ===========================

func TestDelayingType_WaitingLoop_FullCycle(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)

	// Add several items with various delays
	d.AddAfter(1, 50*time.Millisecond)
	d.AddAfter(2, 100*time.Millisecond)
	d.AddAfter(0, 0) // immediate

	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 1 {
		t.Fatalf("expected immediate item added, got %d", mockQ.Len())
	}

	// Step past first delay
	fakeClock.Step(60 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 2 {
		t.Fatalf("expected 2 items after step, got %d", mockQ.Len())
	}

	// Step past second delay
	fakeClock.Step(50 * time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	if mockQ.Len() != 3 {
		t.Fatalf("expected 3 items after final step, got %d", mockQ.Len())
	}

	d.ShutDown()
}

func TestDelayingType_WaitingLoop_HeartbeatAddsExpired(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	// Add with delay shorter than maxWait
	d.AddAfter(42, 5*time.Second)
	time.Sleep(20 * time.Millisecond)

	// Step well past the readyAt via heartbeat
	fakeClock.Step(6 * time.Second)
	time.Sleep(100 * time.Millisecond) // wait for heartbeat to trigger

	if mockQ.Len() != 1 {
		t.Fatalf("expected item to be added via heartbeat, got %d", mockQ.Len())
	}
}

// =========================== Type coverage: test with string type ===========================

func TestDelayingType_WithStringType(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[string]()
	d := newDelayingType[string](mockQ, fakeClock)
	startWaitingLoop(d)
	defer d.ShutDown()

	d.AddAfter("hello", 2*time.Second)
	d.AddAfter("immediate", 0)

	time.Sleep(20 * time.Millisecond)

	if mockQ.Len() != 1 {
		t.Fatalf("expected immediate add, got %d", mockQ.Len())
	}

	fakeClock.Step(2 * time.Second)
	time.Sleep(20 * time.Millisecond)

	if mockQ.Len() != 2 {
		t.Fatalf("expected delayed add, got %d", mockQ.Len())
	}

	items := mockQ.addedItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %v", items)
	}
}

// =========================== Concurrent access tests ===========================

func TestDelayingType_ConcurrentAddAfter(t *testing.T) {
	fakeClock := clocktesting.NewFakeClock(time.Unix(0, 0))
	mockQ := newMockTypedInterface[int]()
	d := newDelayingType[int](mockQ, fakeClock)
	startWaitingLoop(d)

	var wg sync.WaitGroup
	var addCount atomic.Int32

	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				d.AddAfter(base*100+i, 0)
				addCount.Add(1)
			}
		}(g)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	d.ShutDown()

	added := mockQ.addedItems()
	if len(added) != int(addCount.Load()) {
		t.Fatalf("expected %d items, got %d", addCount.Load(), len(added))
	}
}
