package impl

import "sync"

type ConcurrentStrSet struct {
	strings map[string]bool
	sync.Mutex
}

func NewConcurrentStrSet() ConcurrentStrSet{
	strings := make(map[string]bool)
	return ConcurrentStrSet{strings:strings}
}
//func (nbrSet *NbrSet) selectARandomNbrExcept(except string) (string, error) {
//	nbrSet.Lock()
//	defer nbrSet.Unlock()
//	var candidates []string
//	for nbr := range nbrSet.nbrs {
//		if (nbr != except) {
//			candidates = append(candidates,nbr)
//		}
//	}
//	if len(candidates)==0 {
//		return "", errors.New("no valid neighbor was found")
//	}
//	return candidates[rand.Intn(len(candidates))], nil
//}

func (s *ConcurrentStrSet) addStr(newStr string) {
	s.Lock()
	defer s.Unlock()
	s.strings[newStr] = true
}

func (s *ConcurrentStrSet) contains(str string) bool {
	s.Lock()
	defer s.Unlock()
	_,ok := s.strings[str]
	return ok
}

// returns a slice of all strings in the set
func (s *ConcurrentStrSet) getStrSlice() []string {
	s.Lock()
	defer  s.Unlock()
	var res []string
	for str := range s.strings {
		res = append(res, str)
	}
	return res
}

//func (nbrSet *NbrSet) getSize() int {
//	nbrSet.Lock()
//	defer nbrSet.Unlock()
//	return len(nbrSet.nbrs)
//}


