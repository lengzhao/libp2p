package kcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	// "log"
	"time"
)

// KCP A Fast and Reliable ARQ Protocol
type KCP struct {
	conv uint32
	mtu  int

	// wait ack
	writeSegment map[uint8]*tSegment
	readSegment  map[uint8]*tSegment

	windowSize int

	// id in readQueue
	readID uint8
	// write but not receive ack
	writeID uint8
	// first id in writeSegment
	writeAckID uint8

	inBuff []byte

	peerResendFrag uint8
	peerResendTime int64

	interval int64

	// read data
	readQueue []*tSegment
	// need to be sent immediately
	writeQueue     []*tSegment
	waitWriteQueue []*tSegment
	isClosed       bool

	output func(data []byte)
}

type tSegmentHead struct {
	Conv uint32
	Cmd  uint8
	Frag uint8
	Size uint16
}

type tSegment struct {
	tSegmentHead
	Data    []byte
	timeout int64
}

const (
	// CMaxWndSize max window size
	CMaxWndSize = 30
	// CMinWndSize min window size
	CMinWndSize = 3
	// CMaxMtuSize max mtu size
	CMaxMtuSize = 1400
	// CMinMtuSize min mtu size
	CMinMtuSize  = 400
	resendLimit  = 3
	cHeadSize    = 8
	cMinInterval = int64(time.Millisecond * 10)
)

// command of kcp
const (
	ECmdSend = uint8(iota)
	ECmdAck
	ECmdResend
	ECmdReset
	ECmdMTU
	ECmdClose
)

// NewKCP new kcp,kcp not any lock
func NewKCP(conv uint32, cb func([]byte)) *KCP {
	kcp := new(KCP)
	kcp.conv = conv
	kcp.windowSize = resendLimit
	kcp.mtu = CMaxMtuSize - cHeadSize
	kcp.writeSegment = make(map[uint8]*tSegment)
	kcp.readSegment = make(map[uint8]*tSegment)

	kcp.readQueue = make([]*tSegment, 0)
	kcp.writeQueue = make([]*tSegment, 0)
	kcp.interval = cMinInterval * 10

	kcp.output = cb
	return kcp
}

func decode(in []byte, out *tSegmentHead) {
	binary.Read(bytes.NewReader(in), binary.BigEndian, out)
}

// Input receive source data from network
func (k *KCP) Input(data []byte) {
	if len(data) < cHeadSize {
		return
	}
	if len(data) > CMaxMtuSize {
		return
	}
	k.inBuff = data
	for len(k.inBuff) >= cHeadSize {
		sgm := new(tSegment)
		decode(k.inBuff, &sgm.tSegmentHead)
		if sgm.Conv != k.conv {
			return
		}
		// log.Printf("Input %p,id:%d,cmd:%d\n", k, sgm.Frag, sgm.Cmd)
		k.inBuff = k.inBuff[cHeadSize:]
		switch sgm.Cmd {
		case ECmdSend:
			k.receiveSend(sgm)
		case ECmdAck:
			k.receiveAck(sgm)
		case ECmdResend:
			k.receiveResend(sgm)
		case ECmdReset:
			k.receiveReset(sgm)
		case ECmdMTU:
			k.receiveMTU(sgm)
		case ECmdClose:
			k.receiveClose(sgm)
		default:
			k.inBuff = nil
			break
		}
	}
	k.Update()
}

func (k *KCP) receiveSend(send *tSegment) {
	if send.Size > CMaxMtuSize || send.Size == 0 {
		k.inBuff = nil
		return
	}

	//busy
	if len(k.readQueue)+len(k.readSegment) >= CMaxWndSize {
		k.inBuff = k.inBuff[send.Size:]
		return
	}
	//ack
	{
		ack := new(tSegment)
		ack.tSegmentHead = send.tSegmentHead
		ack.Cmd = ECmdAck
		k.writeQueue = append(k.writeQueue, ack)
	}
	if len(k.inBuff) < int(send.Size) {
		k.inBuff = nil
		return
	}
	send.Data = k.inBuff[:send.Size]
	k.inBuff = k.inBuff[send.Size:]

	if send.Frag-k.readID > CMaxWndSize {
		return
	}

	k.readSegment[send.Frag] = send
	for {
		sgm, ok := k.readSegment[k.readID]
		if !ok {
			break
		}
		delete(k.readSegment, k.readID)
		k.readQueue = append(k.readQueue, sgm)
		// log.Printf("receiveSend %p,id:%d,len:%d\n", k, sgm.Frag, len(sgm.Data))
		k.readID++
	}
	if len(k.readSegment) == 0 {
		return
	}
	now := time.Now().UnixNano()
	if k.peerResendFrag != k.readID {
		k.peerResendFrag = k.readID
		if len(k.readSegment) > CMinWndSize {
			k.peerResendTime = 0
		} else {
			k.peerResendTime = now + k.interval
		}
		return
	} else if len(k.readSegment) == CMinWndSize {
		k.peerResendTime = 0
	}
	if k.peerResendTime > now {
		return
	}
	// timeout,must resend
	resend := new(tSegment)
	resend.Conv = k.conv
	resend.Cmd = ECmdResend
	resend.Frag = k.readID
	k.writeQueue = append(k.writeQueue, resend)
	// log.Printf("receiveSend,req resend %p,id:%d,timeout:%d\n", k, resend.Frag, k.peerResendTime)
	k.peerResendTime = now + k.interval*2
}

func (k *KCP) receiveAck(ack *tSegment) {
	if ack.Frag-k.writeAckID > CMaxWndSize {
		return
	}
	sgm, ok := k.writeSegment[ack.Frag]
	if !ok {
		return
	}
	now := time.Now().UnixNano()
	// Update interval
	if now-sgm.timeout < int64(time.Minute) {
		k.interval = (k.interval*19+now-sgm.timeout)/20 + cMinInterval
	}
	delete(k.writeSegment, sgm.Frag)

	for k.writeAckID != k.writeID {
		sgm, ok := k.writeSegment[k.writeAckID]
		if !ok {
			k.writeAckID++
			k.windowSize++
			continue
		}
		// timeout, must resend
		if sgm.timeout <= now {
			k.windowSize = k.windowSize/2 + CMinWndSize
			// log.Printf("ack to resend.id:%d,timeout:%d,new timeout:%d \n", sgm.Frag, sgm.timeout, now+k.interval*2)
			sgm.timeout = now + k.interval*2
			k.writeQueue = append(k.writeQueue, sgm)
		}
		break
	}
	if k.windowSize > CMaxWndSize {
		k.windowSize = CMaxWndSize
	}
}

func (k *KCP) receiveResend(rs *tSegment) {
	sgm, ok := k.writeSegment[rs.Frag]
	if !ok {
		rst := new(tSegment)
		rst.Conv = k.conv
		rst.Cmd = ECmdReset
		rst.Frag = rs.Frag
		k.writeQueue = append(k.writeQueue, rst)
		return
	}
	k.windowSize = k.windowSize/2 + CMinWndSize
	sgm.timeout = time.Now().UnixNano() + k.interval*2
	k.writeQueue = append(k.writeQueue, sgm)
}

func (k *KCP) receiveReset(rs *tSegment) {
	if rs.Frag != k.readID {
		return
	}
	// lost the segment forever
	k.readID++
	for {
		sgm, ok := k.readSegment[k.readID]
		if !ok {
			break
		}
		delete(k.readSegment, k.readID)
		k.readQueue = append(k.readQueue, sgm)
		k.readID++
	}
}

func (k *KCP) receiveMTU(wnd *tSegment) {
	if int(wnd.Size) >= k.mtu {
		return
	} else if wnd.Size < CMinMtuSize {
		return
	}
	k.mtu = int(wnd.Size)
}

func (k *KCP) receiveClose(sgm *tSegment) {
	k.isClosed = true
}

// SetMTU set mtu
func (k *KCP) SetMTU(mtu int) {
	if mtu >= k.mtu {
		return
	} else if mtu < CMinMtuSize {
		return
	}
	k.mtu = mtu
	sgm := new(tSegment)
	sgm.Conv = k.conv
	sgm.Cmd = ECmdMTU
	sgm.Size = uint16(mtu)
	k.writeQueue = append(k.writeQueue, sgm)
}

// IsClosed return true when closed
func (k *KCP) IsClosed() bool {
	return k.isClosed
}

// Close close kcp
func (k *KCP) Close() {
	k.isClosed = true
	sgm := new(tSegment)
	sgm.Conv = k.conv
	sgm.Cmd = ECmdClose
	buff := new(bytes.Buffer)
	binary.Write(buff, binary.BigEndian, sgm.tSegmentHead)
	k.output(buff.Bytes())
}

func (k *KCP) Read(buf []byte) (n int, err error) {
	if k.isClosed {
		err = errors.New("closed")
		return
	}
	if len(k.readQueue) == 0 {
		return
	}
	sgm := k.readQueue[0]
	if len(buf) >= len(sgm.Data) {
		copy(buf, sgm.Data)
		n = len(sgm.Data)
		k.readQueue = k.readQueue[1:]
		return
	}
	n = len(buf)
	copy(buf, sgm.Data[:n])
	sgm.Data = sgm.Data[n:]
	return
}

func (k *KCP) Write(data []byte) (n int, err error) {
	if k.isClosed {
		return 0, errors.New("closed")
	}
	if len(k.writeSegment)+len(k.waitWriteQueue) >= CMaxWndSize {
		return 0, nil
	}

	n = len(data)
	for len(data) > 0 {
		sgm := new(tSegment)
		sgm.Conv = k.conv
		sgm.Cmd = ECmdSend
		sgm.Frag = k.writeID + uint8(len(k.waitWriteQueue))
		if len(data) > k.mtu {
			sgm.Size = uint16(k.mtu)
			sgm.Data = data[:k.mtu]
			data = data[k.mtu:]
		} else {
			sgm.Size = uint16(len(data))
			sgm.Data = data
			data = nil
		}
		// log.Printf("waitWriteQueue %p,id:%d,len:%d\n", k, sgm.Frag, len(sgm.Data))
		k.waitWriteQueue = append(k.waitWriteQueue, sgm)
	}
	k.Update()

	return
}

// Update update to resend/write data
func (k *KCP) Update() error {
	if k.isClosed {
		return errors.New("closed")
	}
	now := time.Now().UnixNano()
	// check write timeout
	if len(k.writeSegment) > 0 {
		sgm := k.writeSegment[k.writeAckID]
		if sgm.timeout < now {
			sgm.timeout = now + k.interval*2
			k.writeQueue = append(k.writeQueue, sgm)
		}
	}
	// check read timeout
	if len(k.readSegment) > 0 {
		if k.peerResendFrag == k.readID {
			if k.peerResendTime <= now {
				k.peerResendTime = now + k.interval*2
				rsd := new(tSegment)
				rsd.Conv = k.conv
				rsd.Cmd = ECmdResend
				rsd.Frag = k.readID
				k.writeQueue = append(k.writeQueue, rsd)
			}
		} else {
			k.peerResendFrag = k.readID
			k.peerResendTime = now + k.interval*2
		}
	}
	// write new data
	for len(k.waitWriteQueue) > 0 && len(k.writeSegment) < k.windowSize {
		sgm := k.waitWriteQueue[0]
		k.waitWriteQueue = k.waitWriteQueue[1:]
		// log.Printf("update to write.id:%d,timeout:%d,new timeout:%d \n", sgm.Frag, sgm.timeout, now+k.interval*2)
		sgm.timeout = now + k.interval*2
		k.writeID++
		k.writeQueue = append(k.writeQueue, sgm)
		k.writeSegment[sgm.Frag] = sgm
	}

	if len(k.writeQueue) == 0 {
		return nil
	}

	//output to write
	queue := k.writeQueue
	k.writeQueue = make([]*tSegment, 0)

	buff := new(bytes.Buffer)
	for _, sgm := range queue {
		// log.Printf("output %p,id:%d,cmd:%d,len:%d,timeout:%d.\n", k, sgm.Frag, sgm.Cmd, len(sgm.Data), sgm.timeout)
		if len(sgm.Data)+buff.Len() > k.mtu {
			k.output(buff.Bytes())
			buff = new(bytes.Buffer)
		}
		binary.Write(buff, binary.BigEndian, sgm.tSegmentHead)
		if sgm.Data != nil {
			buff.Write(sgm.Data)
		}
	}
	k.output(buff.Bytes())

	return nil
}
