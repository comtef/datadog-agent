package netlink

import (
	"encoding/binary"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	ct "github.com/florianl/go-conntrack"
)

const (
	ctaUnspec = iota
	ctaTupleOrig
	ctaTupleReply
)

const (
	ctaTupleIP    = 1
	ctaTupleProto = 2
	ctaTupleZone  = 3
)

const (
	ctaIPv4Src = 1
	ctaIPv4Dst = 2
	ctaIPv6Src = 3
	ctaIPv6Dst = 4
)

const (
	ctaProtoNum     = 1
	ctaProtoSrcPort = 2
	ctaProtoDstPort = 3
)

var scanner = NewAttributeScanner()

// TODO: Replace the intermediate ct.Con object by the same format we use in the cache
func DecodeEvent(e Event) []ct.Con {
	msgs := e.Messages()
	conns := make([]ct.Con, 0, len(msgs))

	for _, msg := range messages {
		c := &ct.Con{}
		scanner.ResetTo(msg.Data)
		err := unmarshalCon(scanner, c)
		if err != nil {
			log.Debugf("error decoding netlink message: %s", err)
			continue
		}
		conns = append(conns, *c)
	}

	// Return buffers to the pool
	e.Done()

	return conns, nil
}

func unmarshalCon(s *AttributeScanner, c *ct.Con) error {
	c.Origin = &ct.IPTuple{}
	c.Reply = &ct.IPTuple{}

	for toDecode := 2; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaTupleOrig:
			toDecode--
			s.Nested(func() error {
				return unmarshalTuple(s, c.Origin)
			})
		case ctaTupleReply:
			toDecode--
			s.Nested(func() error {
				return unmarshalTuple(s, c.Reply)
			})
		}
	}

	return s.Err()
}

func unmarshalTuple(s *AttributeScanner, t *ct.IPTuple) error {
	for toDecode := 2; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaTupleIP:
			toDecode--
			s.Nested(func() error {
				return unmarshalTupleIP(s, t)
			})
		case ctaTupleProto:
			toDecode--
			s.Nested(func() error {
				return unmarshalProto(s, t)
			})
		}
	}
	return s.Err()
}

// TODO: Double check if a message can contain both IPv4 and IPv6 IPs
// We might also want to consider deferring the allocation of the IP byte slice
func unmarshalTupleIP(s *AttributeScanner, t *ct.IPTuple) error {
	for toDecode := 2; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaIPv4Src, ctaIPv6Src:
			toDecode--
			data := copySlice(s.Bytes())
			ip := net.IP(data)
			t.Src = &ip
		case ctaIPv4Dst, ctaIPv6Dst:
			toDecode--
			data := copySlice(s.Bytes())
			ip := net.IP(data)
			t.Dst = &ip
		}
	}

	return s.Err()
}

func unmarshalProto(s *AttributeScanner, t *ct.IPTuple) error {
	t.Proto = &ct.ProtoTuple{}

	for toDecode := 3; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaProtoNum:
			toDecode--
			protoNum := uint8(s.Bytes()[0])
			t.Proto.Number = &protoNum
		case ctaProtoSrcPort:
			toDecode--
			port := binary.BigEndian.Uint16(s.Bytes())
			t.Proto.SrcPort = &port
		case ctaProtoDstPort:
			toDecode--
			port := binary.BigEndian.Uint16(s.Bytes())
			t.Proto.DstPort = &port
		}
	}

	return s.Err()
}

func copySlice(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
