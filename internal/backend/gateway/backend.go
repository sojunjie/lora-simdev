package gateway

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/brocaar/simdev/api/gw"
	"github.com/brocaar/simdev/internal/backend"
	"github.com/brocaar/lorawan"
	"github.com/eclipse/paho.mqtt.golang"
)

const txTopic = "gateway/+/tx"
const uplinkLockTTL = time.Millisecond * 500
const statsLockTTL = time.Millisecond * 500

// Backend implements a MQTT pub-sub backend.
type Backend struct {
	conn            mqtt.Client
	txPacketChan    chan gw.TXPacket
	wg              sync.WaitGroup
}

func NewBackend(server, username, password, cafile string)(backend.Gateway, error){
	b := Backend{
		txPacketChan:	make(chan gw.TXPacket),
	}

	opts := mqtt.NewClientOptions()
	opts.AddBroker(server)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetOnConnectHandler(b.onConnected)
	opts.SetConnectionLostHandler(b.onConnectionLost)

	b.conn = mqtt.NewClient(opts)
	for{
		if token := b.conn.Connect(); token.Wait() && token.Error() != nil{
			time.Sleep(2 * time.Second)
		}else{
			break
		}
	}

	return &b, nil
}

func (b *Backend) Close() error{
	if token := b.conn.Unsubscribe(txTopic); token.Wait() && token.Error != nil{
		return fmt.Errorf("backend/simdev: unsubscribe error %s", token.Error())
	}
	
	b.wg.Wait()
	close(b.txPacketChan)
	return nil
}

// RXPacketChan returns the RXPacket channel.
func (b *Backend) TXPacketChan() chan gw.TXPacket {
	return b.txPacketChan
}

// SendTXPacket sends the given TXPacket to the gateway.
func (b *Backend) SendRXPacket(rxPacket gw.RXPacket) error {
	phyB, err := rxPacket.PHYPayload.MarshalBinary()
	if err != nil {
		return err
	}
	bytes, err := json.Marshal(gw.RXPacketBytes{
		RXInfo:     rxPacket.RXInfo,
		PHYPayload: phyB,
	})
	if err != nil {
		return fmt.Errorf("backend/simdev: rx packet marshal error: %s", err)
	}

	topic := fmt.Sprintf("gateway/%s/rx", rxPacket.RXInfo.MAC)
	fmt.Println("topic %s backend/simdev: publishing rx packet", topic)

	if token := b.conn.Publish(topic, 0, false, bytes); token.Wait() && token.Error() != nil {
		return fmt.Errorf("backend/simdev: publish rx packet failed: %s", token.Error())
	}
	return nil
}

func (b *Backend) txPacketHandler(c mqtt.Client, msg mqtt.Message) {
	b.wg.Add(1)
	defer b.wg.Done()

	fmt.Println("backend/simdev: tx packet received")

	var phy lorawan.PHYPayload
	var txPacketBytes gw.TXPacketBytes
	if err := json.Unmarshal(msg.Payload(), &txPacketBytes); err != nil {
		fmt.Errorf("backend/simdev: unmarshal rx packet error: %s", err)
		return
	}

	if err := phy.UnmarshalBinary(txPacketBytes.PHYPayload); err != nil {
		fmt.Errorf("backend/simdev: unmarshal phypayload error: %s", err)
	}

	b.txPacketChan <- gw.TXPacket{
		TXInfo:     txPacketBytes.TXInfo,
		PHYPayload: phy,
	}
}

func (b *Backend) onConnected(c mqtt.Client) {
	fmt.Println("backend/simdev: connected to mqtt server")
	for {
		fmt.Println("topic %s backend/simdev: subscribing to tx topic", txTopic)
		if token := b.conn.Subscribe(txTopic, 2, b.txPacketHandler); token.Wait() && token.Error() != nil {
			fmt.Errorf("topic %s backend/simdev: subscribe error: %s", txTopic, token.Error())
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

func (b *Backend) onConnectionLost(c mqtt.Client, reason error) {
	fmt.Errorf("backend/simdev: mqtt connection error: %s", reason)
}
