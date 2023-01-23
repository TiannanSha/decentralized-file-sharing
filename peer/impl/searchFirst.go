package impl

import (
	"github.com/rs/xid"
	"github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/types"
	"regexp"
	"strings"
	"time"
)

/* search first and its helpers*/
// first find all matching file, then for each matching file, check if it's a full file
func (n* node) SearchFirst(reg regexp.Regexp, conf peer.ExpandingRing) (name string, err error) {
	// search local for a full file
	threadSafeNames := NewConcurrentStrSet()
	var matchedNames []string
	// search local naming store for file names that match the reg
	n.conf.Storage.GetNamingStore().ForEach(func(name string, val []byte) bool {
		if reg.MatchString(name) {
			matchedNames = append(matchedNames, name)
		}
		return true
	})

	for _,name := range matchedNames {
		if n.checkFilenameIsFullFileLocal(name) {
			return name,nil
		}
	}

	round := 0
	budget := conf.Initial
	// send search requests to nbrs. note conf.Retry is actually number of try...
	for ; round < int(conf.Retry); {
		name, err := n.sendSearchReqToNbrsThenWaitTillTimeout(reg, conf, &threadSafeNames, budget)
		if err != nil {
			log.Error().Msgf("node %s error in searchFirst():", n.addr)
		}
		if (name != "") {
			return name, nil
		}
		round++
		budget *= conf.Factor
	}

	// nothing was found after timeout
	return "", nil
}

// send search requests to nbrs and wait for replies
// returns a name if found a full file in search reply within timeout otherwise empty string
func (n *node) sendSearchReqToNbrsThenWaitTillTimeout(reg regexp.Regexp, conf peer.ExpandingRing, threadSafeNames *ConcurrentStrSet, budget uint)  (string, error) {
	//budget := conf.Initial
	budgets := n.divideBudget(int(budget))
	numNbrs := len(budgets)
	nbrsSent := make(map[string]bool)
	timeoutChanPool := make(map[string]chan bool)  // send stop signal to threads waiting searching reply
	for i:=0; i<numNbrs; i++ {
		// if budget for a nbr is >0, send search requestand wait for reply. budgets can be e.g. [1,0,1,1]
		// or maybe [2,3,2,3]
		if budgets[i]>0 {
			requestId := xid.New().String()
			searchReqMsg := types.SearchRequestMessage{RequestID: requestId, Origin: n.addr,
				Pattern: reg.String(), Budget: uint(budgets[i])}
			transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(searchReqMsg, searchReqMsg.Name())
			nbr,err := n.nbrSet.selectARandomNbrExcept("")
			for ; nbrsSent[nbr]; {
				nbr,err = n.nbrSet.selectARandomNbrExcept("")
			}
			if (err!=nil) {
				// no nbr to send
				log.Warn().Msgf("node %s in searchAll err: %s", n.addr, err)
				return "",err
			}
			// now we found a new nbr that we have not sent request to. send search request to the nbr
			n.directlySendToNbr(transportMsg, nbr)

			// start a thread to wait for the search replies from diff peers in the network
			timeoutChan := make(chan bool, 1)
			timeoutChanPool[requestId] = timeoutChan
			go n.waitForSearchFirstReplyMsg(requestId, threadSafeNames, conf.Timeout, timeoutChan)
		}
	}
	// wait for threads to accumulate threadSafeNames
	time.Sleep(conf.Timeout)
	// notify all threads that time is up, stop collecting replies
	for _,v := range timeoutChanPool {
		v <- true
	}
	// check whether we found some full file
	namesFound := threadSafeNames.getStrSlice()
	if len(namesFound)>0 {
		return namesFound[0],nil
	}
	return "",nil
}

//todo maybe use two sets of channel pools one for search all one for search reply
// todo and when receives a reply, pass to both chan pool
// add full files to threadSafeNames
func (n *node) waitForSearchFirstReplyMsg(requestID string, threadSafeNames *ConcurrentStrSet,
	timeout time.Duration, timeoutChan chan bool) {

	// set a channel to listen whether received a reply msg
	ch := make(chan types.Message, 1)
	n.searchFirstReplyChannels.setAckChannel(requestID, ch)
	//if (n.conf.BackoffDataRequest.Initial==0) {
	//	return
	//}
	log.Info().Msgf("node %s waitForDataReplyaMsg() requestId = %s",n.addr, requestID)
	for {
		//log.Info().Msgf("node %s waitForAck() pktIdWaiting = %s, inside the for loop",n.addr, requestID)
		select {
		case replyMsg := <- ch:
			//if (replyMsg.Name()==types.EmptyMessage{}.Name()) {
			//	// a empty message is sent to notify closing peer, no need to process it, just return an empty datareplymsg
			//	return
			//}
			// received the search reply message
			log.Info().Msgf("node %s recevied search reply for requestId %s", n.addr, requestID)
			searchReplyMsg := replyMsg.(*types.SearchReplyMessage)
			// only append the name if theere's indeed file with filename local on the sender of the reply
			for _,item := range searchReplyMsg.Responses {
				// check if a fileInfo is for a full file
				if checkFileInfoFullFile(item) {
					threadSafeNames.addStr(item.Name)
				}
			}

		case <- timeoutChan:
			// notify by calling thread that we have collected enough time for search replies
			log.Info().Msgf("node %s timed out in wait for search reply", n.addr)
			close(ch)
			n.searchFirstReplyChannels.deleteAckChannel(requestID)
			return
		}
	}
}

func checkFileInfoFullFile(fileinfo types.FileInfo) bool {
	// check whether there's nil chunks
	for _,item := range fileinfo.Chunks {
		if item==nil {
			return false
		}
	}
	return true
}

// get name->metahash->metafile->chunkkeys->check each chunk exists
func (n *node ) checkFilenameIsFullFileLocal(name string) bool {
	metahash := n.conf.Storage.GetNamingStore().Get(name)
	if metahash==nil {
		return false
	}
	metafile := n.conf.Storage.GetDataBlobStore().Get(string(metahash))
	if metafile==nil {
		return false
	}

	// find chunks for metafile
	chunkHashes := metafileToChunkHashes(metafile)
	// check that all chunkHashes are in local blob
	for _, chunkHash := range chunkHashes {
		chunk := n.conf.Storage.GetDataBlobStore().Get(chunkHash)
		if chunk == nil {
			return false
		}
	}
	return true
}

func metafileToChunkHashes(metafile []byte) []string{
	metafileContent := string(metafile)
	return strings.Split(metafileContent, peer.MetafileSep)
}
