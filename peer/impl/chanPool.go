package impl

import (
	"github.com/rs/zerolog/log"
	"go.dedis.ch/cs438/types"
	"sync"
)

type ChanPool struct {
	pktAckChannels map[string]chan bool
	sync.Mutex
}

func NewChanPool() ChanPool {
	channelsMap := make(map[string]chan bool)
	return ChanPool{pktAckChannels: channelsMap}
}

func(c *ChanPool) getAckChannel(pktID string) (chan bool, bool){
	c.Lock()
	defer c.Unlock()
	ch, ok := c.pktAckChannels[pktID]

	return ch,ok
}

func (c *ChanPool) setAckChannel(pktID string, ch chan bool) {
	c.Lock()
	defer c.Unlock()
	c.pktAckChannels[pktID] = ch
}

func (c *ChanPool) deleteAckChannel(pktID string) {
	c.Lock()
	defer c.Unlock()
	delete(c.pktAckChannels, pktID)
}

func(c *ChanPool) notifyAckChannel(pktID string) {
	ch, ok := c.getAckChannel(pktID)
	if !ok {
		return
	}
	ch <- true
}

// stop all go routines that are waiting for an ACK
func (c *ChanPool) stopAllWaitingForACK() {
	log.Info().Msgf("in  stopAllWaitingForACK()")
	c.Lock()
	//log.Info().Msgf("node %s n.pktAckChannels: %s", n.addr, n.pktAckChannels)
	for _,ch := range c.pktAckChannels {
		ch <- true
	}
	//log.Info().Msgf("node %s, end of stopAllWaitingForAck, n.pktAckChannels = %s", n.addr, n.pktAckChannels)
	c.Unlock()
}

type MsgChanPool struct {
	pktAckChannels map[string]chan types.Message
	sync.Mutex
}


/* this type of channel pool has channel that can grab a transport message out of it*/
func NewMsgChanPool() MsgChanPool {
	channelsMap := make(map[string]chan types.Message)
	return MsgChanPool{pktAckChannels: channelsMap}
}

func(c *MsgChanPool) getAckChannel(pktID string) (chan types.Message, bool){
	c.Lock()
	defer c.Unlock()
	ch, ok := c.pktAckChannels[pktID]

	return ch,ok
}

func (c *MsgChanPool) setAckChannel(pktID string, ch chan types.Message) {
	c.Lock()
	defer c.Unlock()
	c.pktAckChannels[pktID] = ch
}

func (c *MsgChanPool) deleteAckChannel(pktID string) {
	c.Lock()
	defer c.Unlock()
	delete(c.pktAckChannels, pktID)
}

func(c *MsgChanPool) passMsgToWaiter(pktID string, msg types.Message) {
	ch, ok := c.getAckChannel(pktID)
	if !ok {
		log.Error().Msgf("in notify ACK channel, no channel found")
	}
	ch <- msg
}

// stop all go routines that are waiting for an ACK
func (c *MsgChanPool) stopAllWaitingForACK() {
	log.Info().Msgf("in  stopAllWaitingForACK()")
	c.Lock()
	//log.Info().Msgf("node %s n.pktAckChannels: %s", n.addr, n.pktAckChannels)
	for _,ch := range c.pktAckChannels {
		ch <- types.EmptyMessage{}
	}
	//log.Info().Msgf("node %s, end of stopAllWaitingForAck, n.pktAckChannels = %s", n.addr, n.pktAckChannels)
	c.Unlock()
}