package sort

// importing fmt and bytes package
import (
	"math/rand"
	"testing"
	"time"
)

// create array
func createArray(num int) []int {
	var array []int
	array = make([]int, num, num)
	rand.Seed(time.Now().UnixNano())
	var i int
	for i = 0; i < num; i++ {
		array[i] = rand.Intn(99999) - rand.Intn(99999)
	}
	return array
}

func TestMergeSorter(t *testing.T) {
	var array []int
	array = createArray(30)
	array = MergeSorter(array)
	for i := 0; i < len(array)-1; i++ {
		if array[i] > array[i+1] {
			t.Error("MergeSorter error")
		}
	}
}
