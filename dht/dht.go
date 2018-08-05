package dht

import (
	"bytes"
	"container/list"
	"encoding/hex"
	"sync"
)

// INodeID interface of node id
type INodeID interface {
	GetPublicKey() []byte
	GetAddresses() string
}

// NodeID struct of node id
type NodeID struct {
	PublicKey []byte
	Address   string
}

// GetPublicKey get public key
func (n *NodeID) GetPublicKey() []byte { return n.PublicKey }

// GetAddresses get address
func (n *NodeID) GetAddresses() string { return n.Address }

const maxBucketLen = 256

func logicDist(id1, id2 INodeID) int {
	k1 := id1.GetPublicKey()
	k2 := id2.GetPublicKey()

	var out = maxBucketLen
	var xor byte
	for i := 0; i < len(k1) && i < len(k2) && i < maxBucketLen/8; i++ {
		xor = k1[i] ^ k2[i]
		out -= 8
		for xor > 0 {
			out++
			xor = xor >> 1
			return out
		}
	}
	return out
}

func equalID(id1, id2 INodeID) bool {
	k1 := id1.GetPublicKey()
	k2 := id2.GetPublicKey()
	return bytes.Compare(k1, k2) == 0
}

// BucketSize defines the NodeID, Key, and routing table data structures.
const BucketSize = 8

// TDHT Distributed Hash Table
type TDHT struct {
	// Current node's ID.
	self    INodeID
	buckets []*tBucket
}

// tBucket holds a list of contacts of this node
type tBucket struct {
	*list.List
	mu *sync.RWMutex
}

func newBucket() *tBucket {
	return &tBucket{
		List: list.New(),
		mu:   &sync.RWMutex{},
	}
}

// CreateDHT create dht
func CreateDHT(id INodeID) *TDHT {
	table := &TDHT{
		self:    id,
		buckets: make([]*tBucket, maxBucketLen),
	}
	for i := 0; i < maxBucketLen; i++ {
		table.buckets[i] = newBucket()
	}

	table.Add(id)

	return table
}

// Self returns the ID of the node hosting the current routing table instance.
func (t *TDHT) Self() INodeID {
	return t.self
}

// Add add the node to dht,return true if new node
func (t *TDHT) Add(target INodeID) bool {
	if len(t.self.GetPublicKey()) != len(target.GetPublicKey()) {
		return false
	}

	bucketID := logicDist(target, t.self)
	bucket := t.getBucket(bucketID)

	// Find current node in bucket.
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	for e := bucket.Front(); e != nil; e = e.Next() {
		if equalID(e.Value.(INodeID), target) {
			return false
		}
	}
	if bucket.Len() <= BucketSize {
		bucket.PushFront(target)
		return true
	}
	return false
}

func hexString(in []byte) string {
	return hex.EncodeToString(in)
}

// GetNodes returns an unique list of all nodes within the routing network (excluding yourself).
func (t *TDHT) GetNodes() (nodes []INodeID) {
	for _, bucket := range t.buckets {
		bucket.mu.RLock()

		for e := bucket.Front(); e != nil; e = e.Next() {
			id := e.Value.(INodeID)
			nodes = append(nodes, id)
		}

		bucket.mu.RUnlock()
	}
	return
}

// RemoveNode removes the node from the routing table.
func (t *TDHT) RemoveNode(target INodeID) bool {
	bucketID := logicDist(target, t.self)
	bucket := t.getBucket(bucketID)

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	for e := bucket.Front(); e != nil; e = e.Next() {
		if equalID(e.Value.(INodeID), target) {
			bucket.Remove(e)
			return true
		}
	}

	return false
}

// NodeExists check if the node exists in the routing table.
func (t *TDHT) NodeExists(target INodeID) bool {
	bucketID := logicDist(target, t.self)
	bucket := t.getBucket(bucketID)

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	for e := bucket.Front(); e != nil; e = e.Next() {
		if equalID(e.Value.(INodeID), target) {
			return true
		}
	}

	return false
}

// Find returns a list of k(count param) nodes with smallest XOR distance
func (t *TDHT) Find(target INodeID) (nodes []INodeID) {
	bucketID := logicDist(target, t.self)
	bucket := t.getBucket(bucketID)

	bucket.mu.RLock()
	for e := bucket.Front(); e != nil; e = e.Next() {
		nodes = append(nodes, e.Value.(INodeID))
	}
	bucket.mu.RUnlock()

	for i := 1; len(nodes) < BucketSize && i < maxBucketLen; i++ {
		if bucketID-i >= 0 {
			other := t.getBucket(bucketID - i)
			other.mu.RLock()
			for e := other.Front(); e != nil; e = e.Next() {
				nodes = append(nodes, e.Value.(INodeID))
			}
			other.mu.RUnlock()
			if len(nodes) >= BucketSize {
				return
			}
		}

		if bucketID+i < maxBucketLen {
			other := t.getBucket(bucketID + i)
			other.mu.RLock()
			for e := other.Front(); e != nil; e = e.Next() {
				nodes = append(nodes, e.Value.(INodeID))
			}
			other.mu.RUnlock()
		}
	}

	return nodes
}

// tBucket returns a specific tBucket by id
func (t *TDHT) getBucket(id int) *tBucket {
	if id >= 0 && id < len(t.buckets) {
		return t.buckets[id]
	}
	return nil
}
