package sort

func QuickSorter[T Ordered](elements []T, below int, upper int) {
	if below < upper {
		var part int
		part = divideParts(elements, below, upper)
		QuickSorter(elements, below, part-1)
		QuickSorter(elements, part+1, upper)
	}
}

func divideParts[T Ordered](elements []T, below int, upper int) int {
	var center T
	center = elements[upper]
	var i int
	i = below
	var j int
	for j = below; j < upper; j++ {
		if elements[j] <= center {
			elements[i], elements[j] = elements[j], elements[i]
			i += 1
		}
	}
	elements[i], elements[upper] = elements[upper], elements[i]
	return i
}
