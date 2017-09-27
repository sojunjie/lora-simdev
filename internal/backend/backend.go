package backend

import  "github.com/brocaar/simdev/api/gw"


type Gateway interface {
	SendRXPacket(gw.RXPacket) error
	TXPacketChan() chan gw.TXPacket
	Close() error
}

