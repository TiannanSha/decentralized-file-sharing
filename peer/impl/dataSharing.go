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


func (n *node) Download(metahash string) ([]byte, error) {
	// get the metafile by first check the local blob store
	// and send to it dataRequestMessage and wait for corresponding dataReplyMessages
	metafile := n.conf.Storage.GetDataBlobStore().Get(metahash)
	if (metafile==nil) {
		// didn't find the metafile at local, send request to a random nbr that has this metafile from catalog
		requestId, transportMsg, randPeer, err := n.sendDataRequestToRandPeerWhoHasHash(metahash)
		if (err != nil) {
			return nil, err
		}

		// wait for data reply msg containing the metafile
		replyMsg, err := n.waitForReplyMsg(requestId, transportMsg, randPeer)
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

		// todo update var metafile, then get hashes outside of if. also what the hack should I return for download? I think I need to store the metafile locally as well and all chunks
	chunkHashes := strings.Split(metafileContent, peer.MetafileSep)
	// for each chunk hash send a request, and then store the replied key value to local
	var allChunks []byte
	for _, chunkHash := range chunkHashes {
		chunk := n.conf.Storage.GetDataBlobStore().Get(chunkHash)
		if (chunk==nil) {
			// need to find chunk at remote
			requestId, transportMsg, randPeer, err := n.sendDataRequestToRandPeerWhoHasHash(chunkHash)
			if (err != nil) {
				return nil, err
			}
			replyMsg, err := n.waitForReplyMsg(requestId, transportMsg, randPeer)
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
			chunk = dataReplyMsg.Value
		}
		// now we have a chunk that is not nil, add it to the result
		allChunks = append(allChunks, chunk...)
	}
	return allChunks, nil
}

func (n *node) sendDataRequestToRandPeerWhoHasHash(hash string) (string, transport.Message, string, error){
	randPeer,err := n.getRandomPeerWhoHasHash(hash)
	if (err!=nil) {
		return "", transport.Message{}, "", errors.New("no randpeer has given hash")
	}
	// send to rand peer dataRequestMessage and wait for corresponding dataReplyMessages
	requestId := xid.New().String()
	dataRequestMsg := types.DataRequestMessage{requestId, hash}
	transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(dataRequestMsg, dataRequestMsg.Name())
	err = n.Unicast(randPeer, transportMsg)
	if err != nil {
		log.Warn().Msgf("err in Download() after unicast : %s", err)
		return "",transportMsg,"",err
	}
	return requestId, transportMsg, randPeer, nil
}

func (n *node) getRandomPeerWhoHasHash(hash string) (string,error) {
	bagOfPeers := n.Catalog.GetCatalog()[hash]
	if (bagOfPeers==nil) {
		return "", errors.New("no peer has given hash")
	}
	randPeerId := rand.Intn(len(bagOfPeers))
	id := 0
	var randPeer string
	for peer,_ := range bagOfPeers {
		if randPeerId==id {
			randPeer = peer
			break
		}
		id++
	}
	return randPeer,nil
}

// todo have a look at wait for ack and probably can reuse chan pool
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
				log.Warn().Msgf("node %s waitForDataReply() requestId %s time out, error when unicast:", n.addr, requestID, err)
				return nil,errors.New("error while unicasting")
			}
			waitTime *= time.Duration(F)
			numResend++
			continue
		}
	}
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

func (cc *ConcurrentCatalog) UpdateConcurrentCatalog(key string, peer string) {
	cc.Lock()
	defer cc.Unlock()

	val := make (map[string]struct{})
	val[peer] = struct{}{}
	cc.catalog[key] = val
}

func (cc *ConcurrentCatalog) GetCatalog() peer.Catalog{
	cc.Lock()
	defer cc.Unlock()
	return cc.catalog
}

/* *** concurrent catalog *** */