// Copyright 2021 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package chunkenc

import (
	"encoding/binary"
	"math"
)

// RLEChunk implements a run-length-encoding chunk that's useful when there are lots of repetitive values stored.
type RLEChunk struct {
	b bstream
}

func NewRLEChunk() *RLEChunk {
	return &RLEChunk{
		b: bstream{
			stream: make([]byte, 4, 128),
			count:  0,
		},
	}
}

// Encoding returns the encoding type.
func (c *RLEChunk) Encoding() Encoding {
	return EncRLE
}

// Bytes returns the underlying byte slice of the chunk.
func (c *RLEChunk) Bytes() []byte {
	return c.b.bytes()
}

// NumSamples returns the number of samples in the chunk.
func (c *RLEChunk) NumSamples() int {
	return int(binary.BigEndian.Uint16(c.Bytes()))
}

func (c *RLEChunk) NumValues() int {
	return int(binary.BigEndian.Uint16(c.Bytes()[2:]))
}

func (c *RLEChunk) Compact() {}

func (c *RLEChunk) Appender() (Appender, error) {
	return &RLEAppender{
		b: &c.b,
	}, nil
}

type RLEAppender struct {
	b *bstream
	v int64
}

func (a *RLEAppender) Append(v int64) {
	num := binary.BigEndian.Uint16(a.b.bytes())
	vals := binary.BigEndian.Uint16(a.b.bytes()[2:])

	// Always append the first value regardless of its value.
	// Then always write the next new value when the values differ.
	// Otherwise, simply increase the count of the current value.
	if num == 0 || a.v != v {
		buf := make([]byte, binary.MaxVarintLen64)
		for _, b := range buf[:binary.PutVarint(buf, v)] {
			a.b.writeByte(b)
		}

		buf = make([]byte, 2)
		binary.BigEndian.PutUint16(buf, 1)
		for _, b := range buf {
			a.b.writeByte(b)
		}
		a.v = v
		binary.BigEndian.PutUint16(a.b.bytes()[2:], vals+1)
	} else {
		b := a.b.bytes()
		// Read the last 3 bytes as the bstream always appends a trailing 0,
		// and we need to two bytes before for the length uint16.
		count := binary.BigEndian.Uint16(b[len(b)-3:])
		binary.BigEndian.PutUint16(a.b.bytes()[len(b)-3:], count+1)
	}

	binary.BigEndian.PutUint16(a.b.bytes(), num+1)
}

func (a *RLEAppender) AppendAt(index uint16, v int64) {
	num := binary.BigEndian.Uint16(a.b.bytes())
	// TODO(metalmatze): We should be able to write sequence of zeros to the stream directly (no loops)
	for i := num; i < index; i++ {
		a.Append(0)
	}
	a.Append(v)
}

// Insert will inset a value into the RLE bytes stream at the given index. If the value is encounted already at the index, it is incremented in place.
func (a *RLEAppender) Insert(index uint16, v int64) {

	it := &rleIterator{
		br:    newBReader(a.b.bytes()[4:]),
		total: binary.BigEndian.Uint16(a.b.bytes()),
		vals:  binary.BigEndian.Uint16(a.b.bytes()[2:]),
	}

	for i := uint16(0); it.Next(); i++ {
		switch {
		case i < index+1: // check the previous index to see if we can simply increment the count
			fallthrough
		case i == index:
			val := it.At()

			totaloffset := it.bsoffset + 4

			if val == v { // we can simply increment the count
				count := binary.BigEndian.Uint16(a.b.bytes()[totaloffset-2:])
				binary.BigEndian.PutUint16(a.b.bytes()[int(totaloffset)-2:], count+1)

				num := binary.BigEndian.Uint16(a.b.bytes())
				binary.BigEndian.PutUint16(a.b.bytes(), num+1)
				return
			}

			if i == index {

				if it.lengthLeft == 0 {
					// dirtry split
					panic("dirty split unsupported")
				}

				remainder := make([]byte, len(a.b.stream)-int(totaloffset-it.vbytes-2))
				copy(remainder, a.b.bytes()[int(totaloffset-it.vbytes-2):len(a.b.stream)-1])
				a.b.stream = a.b.bytes()[:int(totaloffset-it.vbytes-2)]

				buf := make([]byte, binary.MaxVarintLen64)
				for _, b := range buf[:binary.PutVarint(buf, v)] {
					a.b.stream = append(a.b.stream, b)
				}

				buf = make([]byte, 2)
				binary.BigEndian.PutUint16(buf, 1)
				for _, b := range buf {
					a.b.stream = append(a.b.stream, b)
				}

				a.b.stream = append(a.b.stream, remainder...)

				num := binary.BigEndian.Uint16(a.b.bytes())
				binary.BigEndian.PutUint16(a.b.bytes(), num+1)
				vals := binary.BigEndian.Uint16(a.b.bytes()[2:])
				binary.BigEndian.PutUint16(a.b.bytes()[2:], vals+1)

				return
			}
		default:
			continue
		}
	}
}

func (c *RLEChunk) Iterator(it Iterator) Iterator {
	return c.iterator(it)
}

type rleIterator struct {
	br       bstreamReader
	bsoffset uint

	read  uint16
	total uint16

	// stores how often we need to still iterate over the same value
	lengthLeft uint16

	// stores how many different values we have yet to see
	vals uint16

	v      int64
	vbytes uint
	err    error
}

func (c *RLEChunk) iterator(it Iterator) *rleIterator {
	if rleIt, ok := it.(*rleIterator); ok {
		rleIt.Reset(c.b.bytes())
		return rleIt
	}

	return &rleIterator{
		br:    newBReader(c.b.bytes()[4:]),
		total: binary.BigEndian.Uint16(c.b.bytes()),
		vals:  binary.BigEndian.Uint16(c.b.bytes()[2:]),
	}
}

func (it *rleIterator) Next() bool {
	if it.err != nil || it.read == it.total {
		return false
	}

	if it.lengthLeft == 0 {
		v, err := binary.ReadVarint(&it.br)
		if err != nil {
			it.err = err
			return false
		}
		it.v = v

		var b uint
		switch v {
		case 0:
			b = 1
		default:
			b = uint(math.Ceil(math.Log2(float64(v)) / 7))
			if b == 0 {
				b = 1
			}
		}
		it.bsoffset += uint(b)
		it.vbytes = uint(b)

		lengthBytes := make([]byte, 2)
		for i := 0; i < 2; i++ {
			b, err := it.br.ReadByte()
			if err != nil {
				it.err = err
				return false
			}
			lengthBytes[i] = b
		}
		it.bsoffset += 2
		it.vals--
		if it.vals > 0 {
			it.lengthLeft = binary.BigEndian.Uint16(lengthBytes) - 1 // we've already read the first one
		}
		if it.vals == 0 {
			// We can't read the length bytes of the last value, because it may
			// be actively written to, so we infer it from how many samples we
			// know must be left. This is safe because we know this is the last
			// value.
			it.lengthLeft = it.total - it.read
		}
	} else {
		it.lengthLeft--
	}

	it.read++

	return true
}

func (it *rleIterator) Seek(index uint16) bool {
	if it.err != nil {
		return false
	}

	for it.read <= index || it.read == 0 {
		if !it.Next() {
			return false
		}
	}
	return true
}

func (it *rleIterator) At() int64 {
	return it.v
}

func (it *rleIterator) Err() error {
	return it.err
}

func (it *rleIterator) Read() uint64 {
	return uint64(it.read)
}

func (it *rleIterator) Reset(b []byte) {
	// The first 2 bytes contain chunk headers.
	// We skip that for actual samples.
	it.br = newBReader(b[4:])
	it.total = binary.BigEndian.Uint16(b)
	it.vals = binary.BigEndian.Uint16(b[2:])
	it.read = 0

	it.lengthLeft = 0
	it.v = 0
	it.err = nil
}

// FromValuesRLE takes a value and adds it length amounts of times to the Chunk.
// This is mostly helpful in tests.
func FromValuesRLE(value int64, length uint16) Chunk {
	c := NewRLEChunk()
	app, _ := c.Appender()
	for i := 0; i < int(length); i++ {
		app.Append(value)
	}
	return c
}
