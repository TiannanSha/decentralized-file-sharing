package impl

import (
	"errors"
	"math/rand"
	"sync"
)

type NbrSet struct {
	nbrs map[string]bool
	sync.Mutex
}

func (nbrSet *NbrSet) selectARandomNbrExcept(except string) (string, error) {
	nbrSet.Lock()
	defer nbrSet.Unlock()
	var candidates []string
	for nbr := range nbrSet.nbrs {
		if (nbr != except) {
			candidates = append(candidates,nbr)
		}
	}
	if len(candidates)==0 {
		return "", errors.New("no valid neighbor was found")
	}
	return candidates[rand.Intn(len(candidates))], nil
}

func (nbrSet *NbrSet) addNbr(newNbr string) {
	nbrSet.Lock()
	defer nbrSet.Unlock()
	nbrSet.nbrs[newNbr] = true
}

func (nbrSet *NbrSet) contains(node string) bool {
	nbrSet.Lock()
	defer nbrSet.Unlock()
	_,ok := nbrSet.nbrs[node]
	return ok
}

func (nbrSet *NbrSet) getSize() int {
	nbrSet.Lock()
	defer nbrSet.Unlock()
	return len(nbrSet.nbrs)
}

//func (NbrSet *NbrSet) getNRandomPeers(n int) {
//	NbrSet.Lock()
//	defer NbrSet.Unlock()
//	indices := make(map[int]bool)
//	for i:=0; i<n; i++ {
//		index := rand.Intn(n)
//		for ;indices[index]; { // keep generating new rand index and found a new one
//			index = rand.Intn(n)
//		}
//		indices[index] = true // add new index to the indices set
//	}
//
//}


