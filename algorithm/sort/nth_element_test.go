package sort

import (
	"math/rand"
	"testing"
	"time"

	"github.com/andrewbytecoder/gokit/container/slice"
	"github.com/andrewbytecoder/gokit/utils/comparator"
)

func TestNthElement(t *testing.T) {
	a := make([]int, 0)
	b := make([]int, 0)
	rand.Seed(time.Now().UnixNano())
	for i := 0; i < 10; i++ {
		a = append(a, rand.Int()%100)
		b = append(b, a[i])
	}
	sliceA := slice.NewSliceWrapper(a)
	sliceB := slice.NewSliceWrapper(b)
	Sort[int](sliceB.Begin(), sliceB.End(), comparator.IntComparator)

	t.Logf("a: %v\n", a)
	t.Logf("b: %v\n", b)

	for i := 0; i < 2; i++ {
		k := rand.Int() % 10
		NthElement[int](sliceA.Begin(), sliceA.End(), k, comparator.IntComparator)
		t.Logf("%v : %v\n", k, a)
		if b[k] != a[k] {
			t.Errorf("errror")
		}
	}
}
