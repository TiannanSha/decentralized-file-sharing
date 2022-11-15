package impl

import (
	"crypto"
	"encoding/hex"
	"errors"
	"github.com/rs/xid"
	"github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"io"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Upload create and store in the BlobStore the chunks and metafile
func (n* node) Upload(data io.Reader) (metahash string, err error) {

	isFirstChunk := true;
	var MetafileValue string
	var MetafileKeyBytes []byte;
	for {
		buf := make([]byte, n.conf.ChunkSize)
		numBytes, err := data.Read(buf)
		if err == io.EOF {
			// no more bytes to read
			break
		}
		//fmt.Printf("n = %v err = %v buf = %v\n", n, err, buf)
		//fmt.Printf("b[:n] = %q\n", buf[:n])
		chunk := newChunk(buf, numBytes)
		n.conf.Storage.GetDataBlobStore().Set(chunk.hashHexStr, chunk.dataBuf[0:chunk.dataLen])

		// gradually build metafile vale. for first chunk, no separator is needed
		if (isFirstChunk) {
			MetafileValue += chunk.hashHexStr;
			isFirstChunk = false
		} else {
			MetafileValue += (peer.MetafileSep + chunk.hashHexStr)
		}

		// gradually build metafile key bytes
		MetafileKeyBytes = append(MetafileKeyBytes, chunk.hash...)
	}

	// put MetafileValue and its hash in the key value store
	test := hex.EncodeToString(MetafileKeyBytes)
	log.Info().Msgf(test)
	MetafileKey := hashThenEncodeToHexStr(MetafileKeyBytes, len(MetafileKeyBytes))
	n.conf.Storage.GetDataBlobStore().Set(MetafileKey, []byte(MetafileValue))
	return MetafileKey, nil
}

type Chunk struct {
	dataBuf []byte
	hash    []byte
	dataLen    int
	hashHexStr string
}

func newChunk(data []byte, dataLen int) Chunk {
	hash := hashDataToBytes(data, dataLen)
	return Chunk{dataBuf: data, hash: hash, hashHexStr: hex.EncodeToString(hash), dataLen: dataLen}
}

// first do sha256 hash, then do hex encoding
func hashThenEncodeToHexStr(data []byte, dataLen int) string {
	hash := hashDataToBytes(data[0:dataLen], dataLen)
	hashHex := hex.EncodeToString(hash)
	return hashHex
}

func hashDataToBytes(data []byte, dataLen int) []byte {
	h := crypto.SHA256.New()
	h.Write(data[0:dataLen])
	return h.Sum(nil)
}

func (n *node) UpdateCatalog(key string, peer string) {
	n.Catalog.UpdateConcurrentCatalog(key, peer)
}

func (n *node) GetCatalog() peer.Catalog{
	return n.Catalog.GetCatalog()
}


//func (n *node) Download(metahash string) ([]byte, error) {
//	// get the metafile by first check the local blob store
//	// and send to it dataRequestMessage and wait for corresponding dataReplyMessages
//	metafile := n.conf.Storage.GetDataBlobStore().Get(metahash)
//	if (metafile==nil) {
//		// didn't find the metafile at local, send request to a random nbr that has this metafile from catalog
//		requestID, transportMsg, randPeer, err := n.sendDataRequestToRandPeerWhoHasHash(metahash)
//		if (err != nil) {
//			return nil, err
//		}
//
//		// wait for data reply msg containing the metafile
//		replyMsg, err := n.waitForReplyMsg(requestID, transportMsg, randPeer)
//		if (err != nil) {
//			return nil, err
//		}
//		if (replyMsg == nil) {
//			// normal exit when node shut down
//			log.Warn().Msgf("node %s, in download dataReplyMsg==nil", n.addr)
//			return nil, errors.New("replyMsg==nil")
//		}
//
//		// extract the metafile and then the chunk hashes from the
//		dataReplyMsg, ok := replyMsg.(*types.DataReplyMessage)
//		if (!ok) {
//			log.Error().Msg("error when extreact metafile: %s")
//		}
//		if (dataReplyMsg.Value == nil) {
//			return nil, errors.New("dataReplyMsg.Value==nil")
//		}
//		metafile = dataReplyMsg.Value
//		n.conf.Storage.GetDataBlobStore().Set(dataReplyMsg.Key, dataReplyMsg.Value) // store metafile to local
//	}
//
//	// we now have a metafile!=nil, extract chunk hashes from it.
//	metafileContent := string(metafile)
//
//	// update var metafile, then get hashes outside of if.
//	//also what the hack should I return for download? I think I need to store the metafile locally as well and all chunks
//	chunkHashes := strings.Split(metafileContent, peer.MetafileSep)
//	// for each chunk hash send a request, and then store the replied key value to local
//	var allChunks []byte
//	for _, chunkHash := range chunkHashes {
//		chunk := n.conf.Storage.GetDataBlobStore().Get(chunkHash)
//		if (chunk==nil) {
//			// need to find chunk at remote
//			requestID, transportMsg, randPeer, err := n.sendDataRequestToRandPeerWhoHasHash(chunkHash)
//			if (err != nil) {
//				return nil, err
//			}
//			replyMsg, err := n.waitForReplyMsg(requestID, transportMsg, randPeer)
//			if (err != nil) {
//				return nil, err
//			}
//			if (replyMsg == nil) {
//				// normal exit when node shut down
//				log.Warn().Msgf("node %s, in download ReplyMsg==nil", n.addr)
//				return nil, errors.New("replyMsg==nil")
//			}
//			dataReplyMsg, ok := replyMsg.(*types.DataReplyMessage)
//			if (!ok) {
//				log.Error().Msg("error when extreact chunk")
//			}
//			if (dataReplyMsg.Value == nil) {
//				return nil, errors.New("dataReplyMsg.Value==nil")
//			}
//			n.conf.Storage.GetDataBlobStore().Set(dataReplyMsg.Key, dataReplyMsg.Value)
//			chunk = dataReplyMsg.Value
//		}
//		// now we have a chunk that is not nil, add it to the result
//		allChunks = append(allChunks, chunk...)
//	}
//	return allChunks, nil
//}

func (n *node) Download(metahash string) ([]byte, error) {
	// get the metafile by first check the local blob store
	// and send to it dataRequestMessage and wait for corresponding dataReplyMessages
	metafile := n.conf.Storage.GetDataBlobStore().Get(metahash)
	if (metafile==nil) {
		// didn't find the metafile at local, send request to a random nbr that has this metafile from catalog
		requestID, transportMsg, randPeer, err := n.sendDataRequestToRandPeerWhoHasHash(metahash)
		if (err != nil) {
			return nil, err
		}

		// wait for data reply msg containing the metafile
		replyMsg, err := n.waitForReplyMsg(requestID, transportMsg, randPeer)
		if (err != nil) {
			return nil, err
		}
		if (replyMsg == nil) {
			// normal exit when node shut down
			log.Warn().Msgf("node %s, in download dataReplyMsg==nil", n.addr)
			return nil, errors.New("replyMsg==nil")
		}

		// extract the metafile and then the chunk hashes from the
		dataReplyMsg, ok := replyMsg.(*types.DataReplyMessage)
		if (!ok) {
			log.Error().Msg("error when extreact metafile: %s")
		}
		if (dataReplyMsg.Value == nil) {
			return nil, errors.New("dataReplyMsg.Value==nil")
		}
		metafile = dataReplyMsg.Value
		n.conf.Storage.GetDataBlobStore().Set(dataReplyMsg.Key, dataReplyMsg.Value) // store metafile to local
	}

	// we now have a metafile!=nil, extract chunk hashes from it.
	metafileContent := string(metafile)

	// update var metafile, then get hashes outside of if.
	//also what the hack should I return for download? I think I need to store the metafile locally as well and all chunks
	chunkHashes := strings.Split(metafileContent, peer.MetafileSep)
	// for each chunk hash send a request, and then store the replied key value to local
	var allChunks []byte
	var err error
	for _, chunkHash := range chunkHashes {
		chunk := n.conf.Storage.GetDataBlobStore().Get(chunkHash)
		if (chunk==nil) {
			// need to find chunk at remote
			chunk,err = n.getRemoteChunkAndUpdateBlob(chunkHash)
			if (err!=nil) {
				return nil,err
			}
		}
		// now we have a chunk that is not nil, add it to the result
		allChunks = append(allChunks, chunk...)
	}
	return allChunks, nil
}

func (n *node) getRemoteChunkAndUpdateBlob(chunkHash string) ([]byte,error){
	requestID, transportMsg, randPeer, err := n.sendDataRequestToRandPeerWhoHasHash(chunkHash)
	if (err != nil) {
		return nil, err
	}
	replyMsg, err := n.waitForReplyMsg(requestID, transportMsg, randPeer)
	if (err != nil) {
		return nil, err
	}
	if (replyMsg == nil) {
		// normal exit when node shut down
		log.Warn().Msgf("node %s, in download ReplyMsg==nil", n.addr)
		return nil, errors.New("replyMsg==nil")
	}
	dataReplyMsg, ok := replyMsg.(*types.DataReplyMessage)
	if (!ok) {
		log.Error().Msg("error when extreact chunk")
	}
	if (dataReplyMsg.Value == nil) {
		return nil, errors.New("dataReplyMsg.Value==nil")
	}
	n.conf.Storage.GetDataBlobStore().Set(dataReplyMsg.Key, dataReplyMsg.Value)
	chunk := dataReplyMsg.Value
	return chunk,nil
}

//func (n *node) getAllChunksAndUpdateLocalBlob(chunkHashes []string) ([]byte, error){
//	var allChunks []byte
//	for _, chunkHash := range chunkHashes {
//		chunk := n.conf.Storage.GetDataBlobStore().Get(chunkHash)
//		if (chunk==nil) {
//			// need to find chunk at remote
//			requestID, transportMsg, randPeer, err1 := n.sendDataRequestToRandPeerWhoHasHash(chunkHash)
//			if (err1!=nil) {
//				return nil,err1
//			}
//			replyMsg, err2 := n.waitForReplyMsg(requestID, transportMsg, randPeer)
//			if (replyMsg == nil || err2!=nil) {
//				// normal exit when node shut down
//				log.Warn().Msgf("node %s, in download has error", n.addr)
//				return nil, errors.New("error in download")
//			}
//			dataReplyMsg, ok := replyMsg.(*types.DataReplyMessage)
//			if (!ok || dataReplyMsg.Value == nil) {
//				return nil, errors.New("error in download")
//			}
//			n.conf.Storage.GetDataBlobStore().Set(dataReplyMsg.Key, dataReplyMsg.Value)
//			chunk = dataReplyMsg.Value
//		}
//		// now we have a chunk that is not nil, add it to the result
//		allChunks = append(allChunks, chunk...)
//	}
//	return allChunks, nil
//}

func (n *node) sendDataRequestToRandPeerWhoHasHash(hash string) (string, transport.Message, string, error){
	randPeer,err := n.getRandomPeerWhoHasHash(hash)
	if (err!=nil) {
		return "", transport.Message{}, "", errors.New("no randpeer has given hash")
	}
	// send to rand peer dataRequestMessage and wait for corresponding dataReplyMessages
	requestID := xid.New().String()
	dataRequestMsg := types.DataRequestMessage{RequestID: requestID, Key: hash}
	transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(dataRequestMsg, dataRequestMsg.Name())
	err = n.Unicast(randPeer, transportMsg)
	log.Info().Msgf("node %s unicasted to node %s", n.addr, randPeer)
	if err != nil {
		log.Warn().Msgf("err in Download() after unicast : %s", err)
		return "",transportMsg,"",err
	}
	return requestID, transportMsg, randPeer, nil
}

func (n *node) getRandomPeerWhoHasHash(hash string) (string,error) {
	bagOfPeers := n.Catalog.GetCatalog()[hash]
	if (bagOfPeers==nil) {
		return "", errors.New("no peer has given hash")
	}
	randPeerID := rand.Intn(len(bagOfPeers))
	id := 0
	var randPeer string
	for peer := range bagOfPeers {
		if randPeerID==id {
			randPeer = peer
			break
		}
		id++
	}
	return randPeer,nil
}

// exponential backoff, initial wait for I, then I*F, then I*F^2, after sending R messages, stop
// each time send to the same random neighbor
// transportMsg should contain the dataRequestMsg
// the sender should know the type of reply message and cast accordingly
func (n *node) waitForReplyMsg(requestID string, transportMsg transport.Message,
	randPeer string) (types.Message,error) {
	numResend := uint(0) // number of re-send
	I := n.conf.BackoffDataRequest.Initial
	F := n.conf.BackoffDataRequest.Factor
	R := n.conf.BackoffDataRequest.Retry
	waitTime := I
	// set a channel to listen whether received a reply msg
	ch := make(chan types.Message, 1)
	n.dataReplyChannels.setAckChannel(requestID, ch)
	//if (n.conf.BackoffDataRequest.Initial==0) {
	//	return
	//}
	log.Info().Msgf("node %s waitForDataReplyaMsg() requestId = %s",n.addr, requestID)
	for {
		//log.Info().Msgf("node %s waitForAck() pktIdWaiting = %s, inside the for loop",n.addr, requestID)
		select {
		case dataReplyMsg := <- ch:
			if (dataReplyMsg.Name()==types.EmptyMessage{}.Name()) {
				// a empty message is sent to notify closing, no need to process it, just return an empty datareplymsg
				return nil, nil
			}
			// received the data reply message
			log.Info().Msgf("node %s recevied data reply for requestId %s", n.addr, requestID)
			close(ch)
			n.dataReplyChannels.deleteAckChannel(requestID)
			return dataReplyMsg, nil
		case <- time.After(waitTime):
			if (numResend >= R) {
				// already did R retransmit, not going to do another one
				log.Warn().Msgf("still no reply received after R retransimissions")
				return nil,errors.New("no reply after R retransmissions")
			}
			//log.Info().Msgf("node %s waitForAck() pktIdwaiting %s time out", n.addr, requestID)
			err := n.Unicast(randPeer, transportMsg)
			if err != nil {
				log.Warn().Msgf("node %s waitForDataReply() requestId %s time out, error when unicast:%s", n.addr, requestID, err)
				return nil,errors.New("error while unicasting")
			}
			waitTime *= time.Duration(F)
			numResend++
			continue
		}
	}
}

func (n *node) SearchAll(reg regexp.Regexp, budget uint, timeout time.Duration) (names []string, err error) {
	threadSafeNames := NewConcurrentStrSet()

	// search local naming store for file names that match the reg
	n.conf.Storage.GetNamingStore().ForEach(func(name string, val []byte) bool {
		if reg.MatchString(name) {
			threadSafeNames.addStr(name)
		}
		return true
	})

	// divide budget evenly among nbrs and send search request to nbrs to get matching filenames from nbrs
	// wait for search reply until timeout
	budgets := n.divideBudget(int(budget))
	numNbrs := len(budgets)
	nbrsSent := make(map[string]bool)
	timeoutChanPool := make(map[string]chan bool)  // send stop signal to threads waiting searching reply
	requestID := xid.New().String()

	// send search requests to nbrs, with same request id
	for i:=0; i<numNbrs; i++ {
		// if budget for a nbr is >0, send search requestand wait for reply. budgets can be e.g. [1,0,1,1]
		// or maybe [2,3,2,3]
		if budgets[i]>0 {
			searchReqMsg := types.SearchRequestMessage{RequestID: requestID, Origin: n.addr,
				Pattern: reg.String(), Budget: uint(budgets[i])}
			transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(searchReqMsg, searchReqMsg.Name())
			nbr,err := n.nbrSet.selectARandomNbrExcept("")
			for ; nbrsSent[nbr]; {
				nbr,err = n.nbrSet.selectARandomNbrExcept("")
			}
			if (err!=nil) {
				log.Warn().Msgf("node %s in searchAll err: %s", n.addr, err)
			}
			// now we found a new nbr that we have not sent request to. send search request to the nbr
			nbrsSent[nbr] = true
			n.directlySendToNbr(transportMsg, nbr)
			if err != nil {
				return nil, err
			}
		}
	}
	// start a thread to wait for the search replies from diff peers in the network
	timeoutChan := make(chan bool, 1)
	timeoutChanPool[requestID] = timeoutChan
	go n.waitForSearchAllReplyMsg(requestID, &threadSafeNames, timeoutChan)
	time.Sleep(timeout)
	// notify all threads collecting search replies that timeout, let's collect and return
	for _,v := range timeoutChanPool {
		v <- true
	}
	return threadSafeNames.getStrSlice(), nil
}

//func (n *node) SearchAll(reg regexp.Regexp, budget uint, timeout time.Duration) (names []string, err error) {
//	threadSafeNames := NewConcurrentStrSet()
//
//	// search local naming store for file names that match the reg
//	n.conf.Storage.GetNamingStore().ForEach(func(name string, val []byte) bool {
//		if reg.MatchString(name) {
//			threadSafeNames.addStr(name)
//		}
//		return true
//	})
//
//	// divide budget evenly among nbrs and send search request to nbrs to get matching filenames from nbrs
//	// wait for search reply until timeout
//	budgets := n.divideBudget(int(budget))
//	numNbrs := len(budgets)
//	nbrsSent := make(map[string]bool)
//	timeoutChanPool := make(map[string]chan bool)  // send stop signal to threads waiting searching reply
//	for i:=0; i<numNbrs; i++ {
//		// if budget for a nbr is >0, send search requestand wait for reply. budgets can be e.g. [1,0,1,1]
//		// or maybe [2,3,2,3]
//		if budgets[i]>0 {
//			requestID := xid.New().String()
//			searchReqMsg := types.SearchRequestMessage{RequestID: requestID, Origin: n.addr,
//				Pattern: reg.String(), Budget: uint(budgets[i])}
//			transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(searchReqMsg, searchReqMsg.Name())
//			nbr,err := n.nbrSet.selectARandomNbrExcept("")
//			for ; nbrsSent[nbr]; {
//				nbr,err = n.nbrSet.selectARandomNbrExcept("")
//			}
//			if (err!=nil) {
//				log.Warn().Msgf("node %s in searchAll err: %s", n.addr, err)
//			}
//			// now we found a new nbr that we have not sent request to. send search request to the nbr
//			nbrsSent[nbr] = true
//			n.directlySendToNbr(transportMsg, nbr)
//			if err != nil {
//				return nil, err
//			}
//
//			// start a thread to wait for the search replies from diff peers in the network
//			timeoutChan := make(chan bool, 1)
//			timeoutChanPool[requestID] = timeoutChan
//			go n.waitForSearchAllReplyMsg(requestID, &threadSafeNames, timeoutChan)
//		}
//	}
//	time.Sleep(timeout*2)
//	// notify all threads collecting search replies that timeout, let's collect and return
//	for _,v := range timeoutChanPool {
//		v <- true
//	}
//	return threadSafeNames.getStrSlice(), nil
//}


// whenever receives a search reply msg, append the received file name to names
// we wait for timeout amount of time to collect search reply msgs from different peers
func (n *node) waitForSearchAllReplyMsg(requestID string, threadSafeNames *ConcurrentStrSet, timeoutChan chan bool) {

	// set a channel to listen whether received a reply msg
	ch := make(chan types.Message, 1)
	n.searchAllReplyChannels.setAckChannel(requestID, ch)
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
			searchReplyMsg,ok := replyMsg.(*types.SearchReplyMessage)
			if (!ok) {
				log.Warn().Msgf("node %s cast error in wait for search all", n.addr)
			}
			// only append the name if theere's indeed file with filename local on the sender of the reply
			for _,item := range searchReplyMsg.Responses {
				threadSafeNames.addStr(item.Name)
			}

		case <- timeoutChan:
			// notify by calling thread that we have collected enough time for search replies
			log.Info().Msgf("node %s timed out in wait for search reply", n.addr)
			close(ch)
			n.searchAllReplyChannels.deleteAckChannel(requestID)
			return
		}
	}
}

// divide the budget evenly among neighbors. e.g. 4 neighbours, budget 9, then 3 nbr has budget 2, 1 nbr has budget 2+1
func (n *node) divideBudget(budget int) []int {
	numNbrs := n.nbrSet.getSize()
	if numNbrs==0 {
		return nil
	}
	var budgets []int
	for i:=0; i<numNbrs; i++ {
		budgets = append(budgets, budget / numNbrs)
	}
	remainingBudget := budget % numNbrs
	// divide remaining budget evenly to remainingBudget number of nbrs, each increase budget by 1
	// generate remainingBudget many random index from [0 to numNbrs)
	if remainingBudget>0 {
		indices := make(map[int]bool)
		for i:=0; i<remainingBudget; i++ {
			index := rand.Intn(numNbrs)
			for ;indices[index]; { // keep generating new rand index and found a new one
				index = rand.Intn(numNbrs)
			}
			indices[index] = true // add new index to the indices set
		}
		for k := range indices {
			budgets[k]++
		}
	}
	return budgets
}



/* *** filename related *** */

func (n *node) Tag(name string, mh string) error {
	n.conf.Storage.GetNamingStore().Set(name, []byte(mh))
	return nil
}

func (n *node) Resolve(name string) (metahash string) {
	return string(n.conf.Storage.GetNamingStore().Get(name))
}

/* *** concurrent catalog *** */

type ConcurrentCatalog struct {
	catalog peer.Catalog
	sync.Mutex
}

func NewConcurrentCatalog() ConcurrentCatalog{
	catalog := make(map[string]map[string]struct{})
	return ConcurrentCatalog{catalog:catalog}
}

// key is either metahash or chunk hash, peer is the addr of peer that has key's value
func (cc *ConcurrentCatalog) UpdateConcurrentCatalog(key string, peer string) {
	cc.Lock()
	defer cc.Unlock()
	_,keyExists := cc.catalog[key]
	// if key not exists, create a new map as value.
	if (!keyExists) {
		val := make (map[string]struct{})
		val[peer] = struct{}{}
		cc.catalog[key] = val
	} else {
		// if key exists, add the peer to the existing value map
		cc.catalog[key][peer] = struct{}{}
	}
}

func (cc *ConcurrentCatalog) GetCatalog() peer.Catalog{
	cc.Lock()
	defer cc.Unlock()
	return cc.catalog
}

/* *** concurrent catalog *** */