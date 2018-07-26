package kcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"time"
)

// KCP A Fast and Reliable ARQ Protocol
type KCP struct {
	conv int32
	mtu  int

	// wait ack
	writeSegment map[uint8]*tSegment
	readSegment  map[uint8]*tSegment

	windowSize int

	// id in readQueue
	readID uint8
	// in writeSegment but not write
	waitWriteID uint8
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
	writeQueue []*tSegment
	isClosed   bool

	output func(data []byte)
}

type tSegmentHead struct {
	Conv int32
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
	CMaxWndSize = 60
	// CMaxMtuSize max mtu size
	CMaxMtuSize = 1400
	// CMinMtuSize min mtu size
	CMinMtuSize = 400
	resendLimit = 3
	cHeadSize   = 8
)

// command of kcp
const (
	ECmdSend = uint8(iota)
	ECmdAck
	ECmdResend
	ECmdReset
	ECmdWindow
	ECmdClose
)

// NewKCP new kcp,kcp not any lock
func NewKCP(conv int32, cb func([]byte)) *KCP {
	kcp := new(KCP)
	kcp.conv = conv
	kcp.windowSize = resendLimit
	kcp.mtu = CMaxMtuSize
	kcp.writeSegment = make(map[uint8]*tSegment)
	kcp.readSegment = make(map[uint8]*tSegment)

	kcp.readQueue = make([]*tSegment, 0)
	kcp.writeQueue = make([]*tSegment, 0)
	kcp.interval = int64(time.Millisecond) * 10

	kcp.output = cb
	return kcp
}

func encode(in tSegmentHead, out []byte) {
	binary.Write(bytes.NewBuffer(out), binary.BigEndian, in)
}

func decode(in []byte, out *tSegmentHead) {
	binary.Read(bytes.NewReader(in), binary.BigEndian, out)
}

// Input receive source data from network
func (k *KCP) Input(data []byte) {
	if len(data) < cHeadSize {
		return
	}
	if len(data) > k.mtu+cHeadSize {
		return
	}
	k.inBuff = data
	for len(k.inBuff) >= cHeadSize {
		sgm := new(tSegment)
		decode(k.inBuff, &sgm.tSegmentHead)
		if sgm.Conv != k.conv {
			return
		}
		switch sgm.Cmd {
		case ECmdSend:
			k.receiveSend(sgm)
		case ECmdAck:
			k.receiveAck(sgm)
		case ECmdResend:
			k.receiveResend(sgm)
		case ECmdReset:
			k.receiveReset(sgm)
		case ECmdWindow:
			k.receiveWindow(sgm)
		case ECmdClose:
			k.receiveClose(sgm)
		default:
			k.inBuff = nil
			break
		}
	}
	k.update()
}

func (k *KCP) receiveSend(send *tSegment) {
	if send.Size > CMaxMtuSize || send.Size == 0 {
		k.inBuff = nil
		return
	}

	if len(k.readQueue) > CMaxWndSize {
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
		k.readID++
	}
	if len(k.readSegment) == 0 {
		return
	}
	now := time.Now().UnixNano()
	if k.peerResendFrag != k.readID {
		k.readID = k.peerResendFrag
		k.peerResendTime = now
		return
	}
	if k.peerResendTime+k.interval > now {
		return
	}
	// timeout,must resend
	resend := new(tSegment)
	resend.Conv = k.conv
	resend.Cmd = ECmdResend
	resend.Frag = k.readID
	k.writeQueue = append(k.writeQueue, resend)
	k.peerResendTime = now + k.interval
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
	// update interval
	if now-sgm.timeout < int64(time.Minute) {
		k.interval = (k.interval*19+now-sgm.timeout)/20 + 1000
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
			k.windowSize = k.windowSize/2 + 1
			sgm.timeout = now + k.interval
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
		k.writeQueue = append(k.writeQueue)
		return
	}
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

func (k *KCP) receiveWindow(wnd *tSegment) {
	if int(wnd.Size) > k.mtu {
		return
	} else if wnd.Size < CMinMtuSize {
		return
	}
	k.mtu = int(wnd.Size)
}

func (k *KCP) receiveClose(sgm *tSegment) {
	k.isClosed = true
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
	buff := make([]byte, cHeadSize)
	encode(sgm.tSegmentHead, buff)
	k.output(buff)
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
	if len(k.writeSegment) >= k.windowSize {
		return 0, nil
	}

	n = len(data)
	for len(data) > 0 {
		sgm := new(tSegment)
		sgm.Conv = k.conv
		sgm.Cmd = ECmdSend
		sgm.Frag = k.waitWriteID
		k.waitWriteID++
		if len(data) > k.mtu {
			sgm.Size = uint16(k.mtu)
			sgm.Data = data[:k.mtu]
			data = data[k.mtu:]
		} else {
			sgm.Size = uint16(len(data))
			sgm.Data = data
			data = nil
		}
		k.writeSegment[sgm.Frag] = sgm
	}
	k.update()

	return
}

func (k *KCP) update() error {
	if k.isClosed {
		return errors.New("closed")
	}
	now := time.Now().UnixNano()
	// check write timeout
	if len(k.writeSegment) > 0 {
		sgm := k.writeSegment[k.writeAckID]
		if sgm.timeout > now {
			sgm.timeout = now + k.interval
			k.writeQueue = append(k.writeQueue, sgm)
		}
	}
	// check read timeout
	if k.peerResendFrag == k.readID {
		if k.peerResendTime >= now {
			k.peerResendTime = now + k.interval
			rsd := new(tSegment)
			rsd.Conv = k.conv
			rsd.Cmd = ECmdResend
			rsd.Frag = k.readID
			k.writeQueue = append(k.writeQueue, rsd)
		}
	}
	// write new data
	for k.writeID-k.writeAckID < CMaxWndSize && k.writeID < k.waitWriteID {
		sgm := k.writeSegment[k.writeID]
		sgm.timeout = now + k.interval
		k.writeID++
		k.writeQueue = append(k.writeQueue, sgm)
	}

	if len(k.writeQueue) == 0 {
		return nil
	}

	//output to write
	outBuff := make([]byte, k.mtu+cHeadSize)
	buff := outBuff
	for _, sgm := range k.writeQueue {
		if len(sgm.Data)+cHeadSize > len(buff) {
			endOff := len(outBuff) - len(buff)
			outBuff = outBuff[:endOff]
			k.output(outBuff)
			outBuff = make([]byte, k.mtu+cHeadSize)
			buff = outBuff
		}
		encode(sgm.tSegmentHead, buff)
		buff = buff[cHeadSize:]
		if sgm.Data != nil {
			copy(buff, sgm.Data)
			buff = buff[len(sgm.Data):]
		}
	}
	endOff := len(outBuff) - len(buff)
	outBuff = outBuff[:endOff]
	k.output(outBuff)
	k.writeQueue = make([]*tSegment, 0)

	return nil
}
