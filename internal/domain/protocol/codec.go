package protocol

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"ghostwire/internal/domain"
)

const controlCompressThreshold = 256

type DecodedMessage struct {
	Kind    MessageKind
	Flags   uint16
	Payload []byte
}

func EncodeMessage(kind MessageKind, payload []byte, flags uint16) []byte {
	if len(payload) > MaxPayloadSize {
		panic(domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("payload of %d bytes exceeds limit", len(payload))))
	}
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], ProtocolMagic)
	header[4] = ProtocolVersion
	header[5] = byte(kind)
	binary.BigEndian.PutUint16(header[6:8], flags)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(payload)))
	out := make([]byte, HeaderSize+len(payload))
	copy(out, header)
	copy(out[HeaderSize:], payload)
	return out
}

func EncodeJSONMessage(kind MessageKind, v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(domain.WrapError(err, domain.ErrInternal, "failed to encode JSON"))
	}
	jsonBytes := buf.Bytes()

	if len(jsonBytes) > controlCompressThreshold {
		var compressed bytes.Buffer
		w, _ := flate.NewWriter(&compressed, 1)
		w.Write(jsonBytes)
		w.Close()
		if compressed.Len() < len(jsonBytes) {
			return EncodeMessage(kind, compressed.Bytes(), uint16(FlagDeflate))
		}
	}
	return EncodeMessage(kind, jsonBytes, 0)
}

func DecodeControlPayload(msg DecodedMessage) ([]byte, error) {
	if msg.Flags&uint16(FlagDeflate) == 0 {
		return msg.Payload, nil
	}
	r := flate.NewReader(bytes.NewReader(msg.Payload))
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, domain.WrapError(err, domain.ErrProtocol, "failed to inflate control payload")
	}
	if len(out) > 1<<20 {
		return nil, domain.NewError(domain.ErrProtocol, "inflated payload too large")
	}
	return out, nil
}

type MessageDecoder struct {
	chunks   [][]byte
	head     int
	buffered int
}

func (d *MessageDecoder) Feed(chunk []byte) ([]DecodedMessage, error) {
	if len(chunk) > 0 {
		d.chunks = append(d.chunks, chunk)
		d.buffered += len(chunk)
	}

	var messages []DecodedMessage
	for {
		if d.buffered < HeaderSize {
			break
		}
		header := d.peek(HeaderSize)
		magic := binary.BigEndian.Uint32(header[0:4])
		if magic != ProtocolMagic {
			d.reset()
			return nil, domain.NewError(domain.ErrProtocol, "stream is out of sync (bad magic)")
		}
		version := header[4]
		if version != ProtocolVersion {
			d.reset()
			return nil, domain.NewError(domain.ErrVersion,
				fmt.Sprintf("unsupported protocol version %d", version))
		}
		kind := MessageKind(header[5])
		flags := binary.BigEndian.Uint16(header[6:8])
		length := binary.BigEndian.Uint32(header[8:12])

		if int(length) > MaxPayloadSize {
			d.reset()
			return nil, domain.NewError(domain.ErrProtocol,
				fmt.Sprintf("payload length %d exceeds limit", length))
		}
		if d.buffered < HeaderSize+int(length) {
			break
		}
		d.skip(HeaderSize)
		payload := d.take(int(length))
		messages = append(messages, DecodedMessage{Kind: kind, Flags: flags, Payload: payload})
	}
	return messages, nil
}

func (d *MessageDecoder) Reset() {
	d.reset()
}

func (d *MessageDecoder) reset() {
	d.chunks = d.chunks[:0]
	d.head = 0
	d.buffered = 0
}

func (d *MessageDecoder) peek(count int) []byte {
	first := d.chunks[0]
	if len(first)-d.head >= count {
		return first[d.head : d.head+count]
	}
	var parts [][]byte
	need := count
	index := 0
	for need > 0 {
		chunk := d.chunks[index]
		start := 0
		if index == 0 {
			start = d.head
		}
		available := len(chunk) - start
		take := need
		if take > available {
			take = available
		}
		parts = append(parts, chunk[start:start+take])
		need -= take
		index++
	}
	return bytes.Join(parts, nil)
}

func (d *MessageDecoder) skip(count int) {
	d.head += count
	d.buffered -= count
	d.compact()
}

func (d *MessageDecoder) take(count int) []byte {
	first := d.chunks[0]
	if len(first)-d.head >= count {
		out := first[d.head : d.head+count]
		d.head += count
		d.buffered -= count
		d.compact()
		return out
	}
	var parts [][]byte
	need := count
	for need > 0 {
		chunk := d.chunks[0]
		available := len(chunk) - d.head
		take := need
		if take > available {
			take = available
		}
		parts = append(parts, chunk[d.head:d.head+take])
		need -= take
		d.buffered -= take
		d.head += take
		if d.head == len(chunk) {
			d.chunks = d.chunks[1:]
			d.head = 0
		}
	}
	d.compact()
	return bytes.Join(parts, nil)
}

func (d *MessageDecoder) compact() {
	for len(d.chunks) > 0 && d.head >= len(d.chunks[0]) {
		d.head -= len(d.chunks[0])
		d.chunks = d.chunks[1:]
	}
}
