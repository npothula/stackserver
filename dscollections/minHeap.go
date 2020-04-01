package dscollections

import "fmt"

// MinHeap ...
type MinHeap struct {
	heapArray []int64
	size      int64
	maxsize   int64
}

// NewMinHeap ...
func NewMinHeap(maxsize int64) *MinHeap {
	MinHeap := &MinHeap{
		heapArray: []int64{},
		size:      0,
		maxsize:   maxsize,
	}
	return MinHeap
}

func (m *MinHeap) leaf(index int64) bool {
	if index <= m.size && index >= (m.size/2) {
		return true
	}
	return false
}

func (m *MinHeap) parent(index int64) int64 {
	return (index - 1) / 2
}

func (m *MinHeap) leftchild(index int64) int64 {
	return 2*index + 1
}

func (m *MinHeap) rightchild(index int64) int64 {
	return 2*index + 2
}

// Insert ...
func (m *MinHeap) Insert(item int64) error {
	if m.size < m.maxsize {
		m.heapArray = append(m.heapArray, item)
		m.size++
		m.upHeapify(m.size - 1)
	} else {
		if item < m.heapArray[m.size-1] {
			m.heapArray[m.size-1] = item
			m.upHeapify(m.size - 1)
			return nil
		}
		if item < m.heapArray[m.size-2] {
			m.heapArray[m.size-2] = item
			m.upHeapify(m.size - 2)
		}
	}
	return nil
}

func (m *MinHeap) swap(first, second int64) {
	temp := m.heapArray[first]
	m.heapArray[first] = m.heapArray[second]
	m.heapArray[second] = temp
}

func (m *MinHeap) upHeapify(index int64) {
	for m.heapArray[index] < m.heapArray[m.parent(index)] {
		m.swap(index, m.parent(index))
		index = m.parent(index)
	}
}

func (m *MinHeap) downHeapify(current int64) {
	if m.leaf(current) {
		return
	}
	smallest := current
	leftChildIndex := m.leftchild(current)
	rightRightIndex := m.rightchild(current)
	//If current is smallest then return
	if leftChildIndex < m.size && m.heapArray[leftChildIndex] < m.heapArray[smallest] {
		smallest = leftChildIndex
	}
	if rightRightIndex < m.size && m.heapArray[rightRightIndex] < m.heapArray[smallest] {
		smallest = rightRightIndex
	}
	if smallest != current {
		m.swap(current, smallest)
		m.downHeapify(smallest)
	}
	return
}

// Top ...
func (m *MinHeap) Top() int64 {
	return m.heapArray[0]
}

// Search ...
func (m *MinHeap) Search(item int64, ind int64) int64 {
	if item == m.heapArray[ind] {
		return int64(ind)
	}

	leftChildIndex := m.leftchild(ind)
	rightRightIndex := m.rightchild(ind)

	if leftChildIndex < m.size {
		ind := m.Search(item, leftChildIndex)
		if ind != -1 {
			return ind
		}
	}

	if rightRightIndex < m.size {
		ind := m.Search(item, rightRightIndex)
		if ind != -1 {
			return ind
		}
	}
	return -1
}

// Remove ...
func (m *MinHeap) Remove(index int64) int64 {
	top := m.heapArray[index]
	m.heapArray[index] = m.heapArray[m.size-1]
	m.heapArray = m.heapArray[:(m.size)-1]
	m.size--
	m.downHeapify(index)
	return top
}

func mapHeapDemo() {
	topK := int64(100)
	MinHeap := NewMinHeap(topK)

	/*	MinHeap.Insert(4)
		MinHeap.Insert(2)
		MinHeap.Insert(5)
		MinHeap.Insert(8)
		MinHeap.Insert(7)
		MinHeap.Insert(1)
		MinHeap.Insert(3)
	*/
	for j := int64(1); j <= topK; j++ {
		MinHeap.Insert(j)
	}
	fmt.Printf("Min Heap size: %d", MinHeap.size)
	for i := int64(0); i < MinHeap.size; i++ {
		fmt.Println(MinHeap.heapArray[i])
	}
	//fmt.Printf("search: %d\n", MinHeap.Search(9, 0))
	for MinHeap.size > 0 {
		fmt.Printf("removed: %d\n", MinHeap.Remove(0))
	}
}
