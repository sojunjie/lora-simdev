package main

import (
	"crypto/aes"
	"crypto/rand"
	"fmt"
	"bufio"
	"os"

	"github.com/brocaar/simdev/api/gw"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
	"github.com/brocaar/simdev/internal/common"

	gwBackend "github.com/brocaar/simdev/internal/backend/gateway"
)

// getNwkSKey returns the network session key.
func getNwkSKey(appkey lorawan.AES128Key, netID lorawan.NetID, appNonce [3]byte, devNonce [2]byte) (lorawan.AES128Key, error) {
	return getSKey(0x01, appkey, netID, appNonce, devNonce)
}

// getAppSKey returns the application session key.
func getAppSKey(appkey lorawan.AES128Key, netID lorawan.NetID, appNonce [3]byte, devNonce [2]byte) (lorawan.AES128Key, error) {
	return getSKey(0x02, appkey, netID, appNonce, devNonce)
}

func getSKey(typ byte, appkey lorawan.AES128Key, netID lorawan.NetID, appNonce [3]byte, devNonce [2]byte) (lorawan.AES128Key, error) {
	var key lorawan.AES128Key
	b := make([]byte, 0, 16)
	b = append(b, typ)

	// little endian
	for i := len(appNonce) - 1; i >= 0; i-- {
		b = append(b, appNonce[i])
	}
	for i := len(netID) - 1; i >= 0; i-- {
		b = append(b, netID[i])
	}
	for i := len(devNonce) - 1; i >= 0; i-- {
		b = append(b, devNonce[i])
	}
	pad := make([]byte, 7)
	b = append(b, pad...)

	block, err := aes.NewCipher(appkey[:])
	if err != nil {
		return key, err
	}
	if block.BlockSize() != len(b) {
		return key, fmt.Errorf("block-size of %d bytes is expected", len(b))
	}
	block.Encrypt(key[:], b)
	return key, nil
}

func getDevNonce() ([2]byte, error){
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil{
		return b, err
	}
	return b,nil
}

func main(){
	var err error
	common.Band, err = band.GetConfig(band.CN_470_510, false, lorawan.DwellTimeNoLimit)
	if err != nil{
		fmt.Println("Set Band Error")
		return
	}
	
	b, err := gwBackend.NewBackend("tcp://localhost:1883", "loraserver", "loraserver", "")
	if err != nil{
		fmt.Println("new backend failed!")
	}else{
		fmt.Println("new backend success!")
	}
	
	appKey := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	rxInfo := gw.RXInfo{
		MAC : lorawan.EUI64{1,1,1,1,1,1,1,1},
		Frequency: common.Band.UplinkChannels[0].Frequency,
		DataRate: common.Band.DataRates[common.Band.UplinkChannels[0].DataRates[0]],
	}
	
	fmt.Println("rxInfo:", rxInfo)	

	devNonce, err :=  getDevNonce() 
	if err != nil{
		fmt.Println(devNonce)
	}
	joinPayload := lorawan.PHYPayload{
		MHDR: lorawan.MHDR{
			MType: lorawan.JoinRequest,
			Major: lorawan.LoRaWANR1,
		},
		MACPayload: &lorawan.JoinRequestPayload{
			AppEUI: [8]byte{1,2,3,4,5,6,7,8},
			DevEUI: [8]byte{2,3,4,5,6,7,8,9},
			DevNonce: devNonce,
		},
	}
	
	fmt.Println("JoinPayload:", joinPayload)

	joinPayload.SetMIC(appKey)

	fmt.Println("SetMIC")

	jrBytes, err := joinPayload.MarshalBinary()
	if err != nil{
		fmt.Errorf("joinPayload.MarshalBinary failed!")
	}else{
		fmt.Println(jrBytes)
	}

	var rxPacket = gw.RXPacket{
		RXInfo : rxInfo,
		PHYPayload : joinPayload,
	}

	fmt.Println("rxPacket:", rxPacket)

	if err := b.SendRXPacket(rxPacket); err != nil{
		fmt.Errorf("SendRXPacket failed")
	}

	txPacket := <- b.TXPacketChan()
	
	jaPhy := txPacket.PHYPayload

	if err := jaPhy.DecryptJoinAcceptPayload(appKey); err != nil{
		fmt.Errorf("DecryptJoinAcceptPayload failed! %s\n", err)
	}

	fmt.Println(jaPhy)

	joinAccept, ok := jaPhy.MACPayload.(*lorawan.JoinAcceptPayload)
	if !ok{
		fmt.Errorf("join-accept PHYPayload dose not contain a JoinAcceptPayload")
	}

	nwkSKey, err := getNwkSKey(appKey, joinAccept.NetID, joinAccept.AppNonce, devNonce)
	if err != nil {
		fmt.Errorf("getNwkSKey fail %s", err)
	}

	fmt.Println("nwkSKey: %s", nwkSKey)

	appSKey, err := getAppSKey(appKey, joinAccept.NetID, joinAccept.AppNonce, devNonce)
	if err != nil {
		fmt.Errorf("getAppSKey fail %s", err)
	}

	fmt.Println("appSKey: %s", appSKey)


	//send data
	var FPort byte = 0x01
	var FCnt uint32 = 0x01

	inputReader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("Please Input Message:")
		input, err := inputReader.ReadString('\n')
		
		fmt.Println("AppSKey:", appSKey)
		fmt.Println("DevAddr:", joinAccept.DevAddr)
		fmt.Println("FCnt:", FCnt)
		fmt.Println("Message: ", input ,[]byte(input))
		data, err:= lorawan.EncryptFRMPayload(appSKey, true, joinAccept.DevAddr, FCnt, []byte(input))
		fmt.Println("Encrypted Data:", data)
		if err != nil {
			fmt.Errorf("EncryptFRMPayload failed")
		}

		dataPayload := lorawan.PHYPayload{
			MHDR: lorawan.MHDR{
 				MType: lorawan.UnconfirmedDataUp,
 				Major: lorawan.LoRaWANR1,
 			},
 			MACPayload:  &lorawan.MACPayload{
 				FHDR: lorawan.FHDR{
					DevAddr: joinAccept.DevAddr,
					FCtrl: lorawan.FCtrl{},
					FCnt: FCnt,
				},
				FPort: &FPort,
 				FRMPayload: []lorawan.Payload{
					&lorawan.DataPayload{Bytes: data,},
				},
			},
		}
		

		dataPayload.SetMIC(nwkSKey)

		dataBytes, err := dataPayload.MarshalBinary()
		if err != nil{
			fmt.Errorf("dataPayload.MarshalBinary failed!")
		}else{
			fmt.Println(dataBytes)
		}

		var dataPacket = gw.RXPacket{
			RXInfo : rxInfo,
			PHYPayload : dataPayload,
		}

		//fmt.Println("dataPacket:", dataPacket)

		if err := b.SendRXPacket(dataPacket); err != nil{
			fmt.Errorf("SendDataPacket failed")
		}
		FCnt++
	}	
}
