package impl

import (
	"encoding/json"
	"github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/peer"
	"go.dedis.ch/cs438/transport"
	"go.dedis.ch/cs438/types"
	"golang.org/x/xerrors"
	"math/rand"
	"regexp"
	"strings"
)

func (n* node) ExecRumorsMessage(msg types.Message, pkt transport.Packet) error {
	log.Info().Msgf("for node %s,  Enter ExecRumorsMessage()", n.addr)
	// cast the message to its actual type. You assume it is the right type.
	rumorsMsg, ok := msg.(*types.RumorsMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}
	log.Info().Msgf("node %s, in ExecRumorsMessage, parse out rumorsMsg:%s",n.addr , rumorsMsg)

	/* do your stuff here with rumorsMsg... */
	// Process each Rumor Ʀ by checking if Ʀ is expected or not
	atLeastOneRumorExpected := false
	for _,r:=range rumorsMsg.Rumors {
		if n.ExecRumor(r, pkt) {
			atLeastOneRumorExpected = true
		}
	}

	// if one rumor is expected need to send rumorsMsg to another random neighbor
	// when you spread a rumor the source should still be the original src
	if atLeastOneRumorExpected {
		msg:= n.wrapInTransMsgBeforeUnicastOrSend(rumorsMsg, rumorsMsg.Name())
		nbr,err := n.nbrSet.selectARandomNbrExcept(pkt.Header.Source)
		if (err!=nil) {
			// no suitable neighbour, don't send
			log.Warn().Msgf("node %s error in ExecRumorsMessage():%s", n.addr, err)
		} else {
			err := n.Relay(pkt.Header.Source, nbr, msg)
			if err != nil {
				log.Warn().Msgf("node %s , error in ExecRumorsMessage() after Relay() :%s", n.addr, err)
			}
		}
	}

	// Send back an AckMessage to the relayby. need to add src to my routing table
	// if this is my own message, no need to reply ack
	src := pkt.Header.Source
	if (src==n.addr) {
		return nil
	}
	//n.SetRoutingEntry(src, pkt.Header.RelayedBy)
	ackMsg := types.AckMessage{Status: n.Status.getStatusMsg(), AckedPacketID: pkt.Header.PacketID}
	data,err := json.Marshal(ackMsg)
	if (err!=nil) {
		log.Warn().Msgf("node %s, err in ExecRumorsMessage: %s", n.addr, err)
	}
	transportMsg := transport.Message{Type: ackMsg.Name(), Payload: data}
	//transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(ackMsg, ackMsg.Name())
	n.directlySendToNbr(transportMsg, pkt.Header.RelayedBy)

	// used to be unicast ack to src
	//err = n.Unicast(src, transportMsg)
	//if err != nil {
	//	log.Warn().Msgf("node %s , err in ExecRumorsMessage() when calling unicast: %s", n.addr, err)
	//}

	return nil
}

// return whether this rumor message was expected
func (n *node) ExecRumor(rumor types.Rumor, pkt transport.Packet) bool {
	//currSeqNum, _ := n.StatusMsg[rumor.Origin]
	currSeqNum := n.Status.getSeqForNode(rumor.Origin)
	if (currSeqNum+1) == rumor.Sequence {
		pktInRumor := n.wrapMsgIntoPacket(*rumor.Msg, pkt)
		err := n.conf.MessageRegistry.ProcessPacket(pktInRumor)
		if err != nil {
			log.Warn().Msgf("node %s in ExecRumor(), ProcessPacket returns error: %s", n.addr, err)
		}
		//n.StatusMsg[rumor.Origin] = currSeqNum+1
		//n.Status.incrementSeqForNode(rumor.Origin)
		//n.rumorsReceived[rumor.Origin] = append(n.rumorsReceived[rumor.Origin], rumor)
		n.Status.updateForNewRumorFromNode(rumor.Origin, rumor)
		originIsNbr := n.nbrSet.contains(rumor.Origin)
		if !originIsNbr {
			n.SetRoutingEntry(rumor.Origin, pkt.Header.RelayedBy)
		}
		return true
	}
	return false
}

// The handler. This function will be called when a chat message is received.
func (n* node) ExecChatMessage(msg types.Message, pkt transport.Packet) error {
	// cast the message to its actual type. You assume it is the right type.
	chatMsg, ok := msg.(*types.ChatMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}

	// do your stuff here with chatMsg...
	// log that message. Nothing else needs to be done. The ChatMessage is parsed
	// by the web-frontend using the message registry.
	log.Info().Msgf("chatMsg:%s",chatMsg)
	return nil
}

func (n* node) ExecAckMessage(msg types.Message, pkt transport.Packet) error {
	// cast the message to its actual type. You assume it is the right type.
	ackMsg, ok := msg.(*types.AckMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}

	// notify the go routine waiting for pktId that we received ack for it
	n.pktAckChannels.notifyAckChannel(ackMsg.AckedPacketID)

	// process the status message inside the ack message
	statusMsg := n.wrapInTransMsgBeforeUnicastOrSend(ackMsg.Status, ackMsg.Status.Name())
	newPkt := n.wrapMsgIntoPacket(statusMsg, pkt)
	err := n.conf.MessageRegistry.ProcessPacket(newPkt)
	if err != nil {
		log.Info().Msgf("node %s err in ExecAckMessage(): %s", n.addr, err)
	}

	return nil
}

func (n *node) ExecPrivateMessage(msg types.Message, pkt transport.Packet) error {
	privateMsg, ok := msg.(*types.PrivateMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}
	_,IamRecipient := privateMsg.Recipients[n.addr]
	if IamRecipient {
		newPkt :=n.wrapMsgIntoPacket(*privateMsg.Msg, pkt)
		err := n.conf.MessageRegistry.ProcessPacket(newPkt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (n *node) ExecStatusMessage(msg types.Message, pkt transport.Packet) error {
	log.Info().Msgf("**** !@##@node %s enters ExecStatusMessage()", n.addr)
	statusMsg, ok := msg.(*types.StatusMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}

	IHaveNew, otherHasNew, rumorsNeedToSend := n.Status.compareWithOthersStatus(statusMsg)

	//log.Info().Msgf("**** !@##@node %s enters ExecStatusMessage(), " +
	//	"otherHasNew=%s, IHaveNew=", n.addr, otherHasNew, IHaveNew)
	if otherHasNew {
		// I have less than this node, so I send my status to this node and it will send back more messages
		msg := n.wrapInTransMsgBeforeUnicastOrSend(n.Status.getStatusMsg(), n.Status.getStatusMsg().Name())
		n.directlySendToNbr(msg, pkt.Header.Source)
	}
	if IHaveNew {
		// send the accumulated rumors as a rumorsMsg to the peer
		rumorsMessage := types.RumorsMessage{Rumors: rumorsNeedToSend}
		msgToUnicast := n.wrapInTransMsgBeforeUnicastOrSend(rumorsMessage, rumorsMessage.Name())
		n.directlySendToNbr(msgToUnicast, pkt.Header.Source)  // to do should ttl be 0 ?
	}
	if !otherHasNew && !IHaveNew &&  rand.Float64() < n.conf.ContinueMongering{
		// me and nbr have same status
		// With a certain probability, peer P sends a status message to a random neighbor,
		// different from the one it received the status from.

		newNbr,err := n.nbrSet.selectARandomNbrExcept(pkt.Header.Source)
		if (err!=nil) {
			log.Warn().Msgf("Node %s,In ExecStatusMessage, err: %s", n.addr, err)
		}
		if newNbr!="" {
			// successfully get a random nbr
			statusMsg := n.wrapInTransMsgBeforeUnicastOrSend(n.Status.getStatusMsg(),
				n.Status.getStatusMsg().Name())
			n.directlySendToNbr(statusMsg, newNbr)
		}

	}
	return nil
}

func (n* node) ExecDataReplyMessage(msg types.Message, pkt transport.Packet) error {
	// cast the message to its actual type. You assume it is the right type.
	dataReplyMsg, ok := msg.(*types.DataReplyMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}

	n.dataReplyChannels.passMsgToWaiter(dataReplyMsg.RequestID, msg) // pass the reply msg to the thread waiting
	return nil
}

func (n* node) ExecDataRequestMessage(msg types.Message, pkt transport.Packet) error {
	// cast the message to its actual type. You assume it is the right type.
	dataRequestMsg, ok := msg.(*types.DataRequestMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}

	// reply a data replyMessage to the sender of data request
	dataReplyMsg := types.DataReplyMessage{
		RequestID: dataRequestMsg.RequestID,
		Key:       dataRequestMsg.Key,
		Value:     n.conf.Storage.GetDataBlobStore().Get(dataRequestMsg.Key),
	}
	transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(dataReplyMsg, dataReplyMsg.Name())
	log.Info().Msgf("node %s unicast to addr %s", n.addr, pkt.Header.Source )
	err := n.Unicast(pkt.Header.Source, transportMsg)
	if err != nil {
		return err
	}
	return nil
}

func (n* node) ExecSearchReplyMessage(msg types.Message, pkt transport.Packet) error {
	// cast the message to its actual type. You assume it is the right type.
	searchReplyMsg, ok := msg.(*types.SearchReplyMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}
	log.Info().Msgf("node %s received search reply msg: %s", n.addr, searchReplyMsg)

	// update the naming store and the catalog based on the message content
	for _,fileinfo := range searchReplyMsg.Responses {
		n.conf.Storage.GetNamingStore().Set(fileinfo.Name, []byte(fileinfo.Metahash))
		// add metahash to catalog
		n.Catalog.UpdateConcurrentCatalog(fileinfo.Metahash, pkt.Header.Source)
		// add chunks to catalog
		for _,chunkKey := range fileinfo.Chunks {
			if (chunkKey!=nil) {
				n.Catalog.UpdateConcurrentCatalog(string(chunkKey), pkt.Header.Source)
			}
		}
	}

	// reply a data replyMessage to the sender of data request
	//requestID := searchReplyMsg.RequestID
	//log.Info().Msgf("node %s n.searchAllReplyChannels: %s", n.addr, n.searchAllReplyChannels.pktAckChannels)
	//ch,_ := n.searchFirstReplyChannels.getAckChannel(requestID)

	n.searchAllReplyChannels.passMsgToWaiter(searchReplyMsg.RequestID, msg)

	//log.Info().Msgf("node %s n.searchFirstReplyChannels: %s", n.addr, n.searchFirstReplyChannels)
	n.searchFirstReplyChannels.passMsgToWaiter(searchReplyMsg.RequestID, msg)
	return nil
}

//
func (n* node) ExecSearchRequestMessage(msg types.Message, pkt transport.Packet) error {
	log.Info().Msgf("node %s in ExecSearchRequestMessage",n.addr)
	// cast the message to its actual type. You assume it is the right type.
	searchRequestMsg, ok := msg.(*types.SearchRequestMessage)
	if !ok {
		return xerrors.Errorf("wrong type: %T", msg)
	}

	// check if we received this request before

	if (n.searchRequestsReceived.contains(searchRequestMsg.RequestID)) {
		return nil
	} else {
		// this is new request
		n.searchRequestsReceived.addStr(searchRequestMsg.RequestID)
	}

	// forward search request msgs if there's remaining budget
	budget := searchRequestMsg.Budget - 1
	if budget>0 {
		budgets := n.divideBudget(int(budget))
		numNbrs := len(budgets)
		nbrsSent := make(map[string]bool)
		for i:=0; i<numNbrs; i++ {
			// if budget>0, send search request to a new nbr. budgets can be e.g. [1,0,1,1]
			// or maybe [2,3,2,3]
			if budgets[i]>0 {
				searchReqMsg := types.SearchRequestMessage{
					RequestID: searchRequestMsg.RequestID,
					Origin: searchRequestMsg.Origin,
					Pattern: searchRequestMsg.Pattern,
					Budget: uint(budgets[i]),
				}
				transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(searchReqMsg, searchReqMsg.Name())
				// find an unseen nbr. also it needs not be the packet src。 may be should exclude origin of search
				nbr,err := n.nbrSet.selectARandomNbrExcept(pkt.Header.Source)
				for ; nbrsSent[nbr]; {
					nbr,err = n.nbrSet.selectARandomNbrExcept(pkt.Header.Source)
				}
				if (err!=nil) {
					log.Warn().Msgf("node %s in ExecSearchRequestMessage: %s", n.addr, err)
				}
				// now we found a new nbr that we have not sent request to. send search request to the nbr
				n.directlySendToNbr(transportMsg, nbr) // this auto changes the packet's src and relayby field
			}
		}
	}

	// Check its naming store for any matching name and then construct a types.FilesInfo{} for each file mapped by a
	// matching name. Include a matching name only if the peer has the corresponding metafile in its blob store
	reg := regexp.MustCompile(searchRequestMsg.Pattern)
	var fileInfos []types.FileInfo
	// loop thru naming store and check for filename matching the regex
	n.conf.Storage.GetNamingStore().ForEach(func(name string, val []byte) bool {
		if reg.MatchString(name) {
			// include a matching name only if the peer has corresponding metafile in its blob store
			//name->metahash->metafile->chunk keys->chunks
			metahash := string(val)
			metafile := n.conf.Storage.GetDataBlobStore().Get(metahash)
			if (metafile!=nil) {
				// find chunks for metafile
				metafileContent := string(metafile)
				chunkHashes := strings.Split(metafileContent, peer.MetafileSep)
				var allChunks [][]byte
				// append a chunkhash if it's in local store, otherwise append nil
				for _, chunkHash := range chunkHashes {
					chunk := n.conf.Storage.GetDataBlobStore().Get(chunkHash)
					if chunk!= nil {
						allChunks = append(allChunks, []byte(chunkHash))
					} else {
						allChunks = append(allChunks, nil)
					}
				}

				fileInfo := types.FileInfo{
					Name:     name,
					Metahash: metahash,
					Chunks:   allChunks,
				}
				fileInfos = append(fileInfos, fileInfo)
			}
		}
		return true
	})

	// send search reply msg to the src of the pkt, but change the destination to searchRequestMsg.origin
	searchReplyMsg := types.SearchReplyMessage{
		RequestID: searchRequestMsg.RequestID,
		Responses: fileInfos,
	}
	transportMsg := n.wrapInTransMsgBeforeUnicastOrSend(searchReplyMsg, searchReplyMsg.Name())
	log.Info().Msgf("in node %s, before send searchReply to orgin %s", n.addr, searchRequestMsg.Origin)
	n.sendToNbrAndChangeDest(transportMsg, pkt.Header.Source, searchRequestMsg.Origin)

	return nil
}

/**
 * this function does not use routing table but directly use Send() to send back to nbr
 * @return packet it sent
 */
func (n *node) directlySendToNbr(msgToReply transport.Message, nbr string) transport.Packet {
	header := transport.NewHeader(n.addr, n.addr, nbr, 0)
	newPkt := transport.Packet{
		Header: &header,
		Msg:    &msgToReply,
	}
	err := n.conf.Socket.Send(nbr, newPkt, 0)
	if err != nil {
		log.Warn().Msgf("node %s, send to nbr %s, in directlySendToNbr() err: %s", n.addr,nbr, err)
		log.Warn().Msgf("in directlySendToNbr() err: %s", err)
	}
	return newPkt
}

func (n *node) sendToNbrAndChangeDest(msgToReply transport.Message, nbr string, dest string) transport.Packet {
	header := transport.NewHeader(n.addr, n.addr, dest, 0)
	newPkt := transport.Packet{
		Header: &header,
		Msg:    &msgToReply,
	}
	err := n.conf.Socket.Send(nbr, newPkt, 0)
	if err != nil {
		log.Warn().Msgf("node %s, send to nbr %s, in directlySendToNbr() err: %s", n.addr,nbr, err)
		log.Warn().Msgf("in directlySendToNbr() err: %s", err)
	}
	return newPkt
}