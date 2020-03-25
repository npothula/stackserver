package dscollections

// Stack is a LIFO (Last in first out) data structure implementation.
// It is based on a deque container and focuses its API on core
// functionalities: Push, Pop, Top, Size, Empty. Every operations time complexity
// is O(1).
//
// As it is implemented using a Deque container, every operations
// over an instiated Stack are synchronized and safe for concurrent
// usage.
type Stack struct {
	*Deque
}

// NewStack constructs the stack object and returns it
func NewStack() *Stack {
	return &Stack{
		Deque: NewDeque(),
	}
}

// NewCappedStack constructs the stack object with the specified capacity limit.
func NewCappedStack(capacity int) *Stack {
	return &Stack{
		Deque: NewCappedDeque(capacity),
	}
}

// Push adds on an item on the top of the Stack
func (s *Stack) Push(item interface{}) {
	s.Deque.Prepend(item)
}

// Pop removes and returns the item on the top of the Stack
func (s *Stack) Pop() interface{} {
	return s.Deque.Shift()
}

// Top returns the item on the top of the stack
func (s *Stack) Top() interface{} {
	return s.Deque.First()
}
