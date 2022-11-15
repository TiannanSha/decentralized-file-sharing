package impl
//
//import "sync"
//
//type ThreadSafeSlice struct {
//	slice []string
//	sync.Mutex
//}

//func (s *ThreadSafeSlice) append(newStr string) {
//	s.Lock()
//	defer s.Unlock()
//	s.slice = append(s.slice, newStr)
//}

//func (s *ThreadSafeSlice) getSlice() []string{
//	s.Lock()
//	defer s.Unlock()
//	return s.slice
//}

//func NewEmptyThreadSafeSlice() ThreadSafeSlice{
//	var slice []string
//	return ThreadSafeSlice{slice: slice}
//}