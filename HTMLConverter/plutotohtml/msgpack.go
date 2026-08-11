package plutotohtml

import (
	"errors"
	"fmt"
	"math"
)

// MsgPack format tags, per the specification. Named rather than inlined so the decoder reads as a
// table of formats instead of a wall of hex.
const (
	tagNil       = 0xc0
	tagFalse     = 0xc2
	tagTrue      = 0xc3
	tagBin8      = 0xc4
	tagBin32     = 0xc6
	tagExt8      = 0xc7
	tagExt32     = 0xc9
	tagFloat32   = 0xca
	tagFloat64   = 0xcb
	tagUint8     = 0xcc
	tagUint64    = 0xcf
	tagInt8      = 0xd0
	tagInt64     = 0xd3
	tagFixExt1   = 0xd4
	tagFixExt16  = 0xd8
	tagStr8      = 0xd9
	tagStr32     = 0xdb
	tagArray16   = 0xdc
	tagArray32   = 0xdd
	tagMap16     = 0xde
	tagMap32     = 0xdf
	maxFixInt    = 0x7f
	minFixMap    = 0x80
	maxFixMap    = 0x8f
	minFixArray  = 0x90
	maxFixArray  = 0x9f
	minFixStr    = 0xa0
	maxFixStr    = 0xbf
	minNegFixInt = 0xe0
	fixMapMask   = 0x0f
	fixStrMask   = 0x1f

	bitsPerByte  = 8
	float32Bytes = 4
	float64Bytes = 8
	int64Bits    = 64
	// The 16-bit array/map headers carry a two-byte length; the 32-bit ones twice that.
	wideLenBytes = 2
)

// Pluto does not use MsgPack's own `bin` family for byte arrays. Instead it registers Julia's
// typed arrays as extension types, numbering them by their position in
// [Int8, UInt8, Int16, UInt16, Int32, UInt32, Float32, Float64] offset by 0x10 (see
// SpaceStation.jl/src/webserver/MsgPack.jl). A PNG body therefore arrives as ext 0x12, a
// Vector{UInt8}. Decoding it as an opaque extension is the single easiest way to end up with
// silently broken images, so the byte-array case is unwrapped here.
const extUint8 = 0x12

// Ext is an extension-typed value that has no natural Go equivalent. Byte arrays (ext 0x12) are
// unwrapped to []byte instead of arriving as an Ext.
type Ext struct {
	Type byte
	Data []byte
}

// maxDecodeDepth bounds recursion on hostile or corrupt input. Pluto's own tree viewer stops
// emitting nested elements past depth 3 (tree viewer.jl), so real notebooks stay far below this.
const maxDecodeDepth = 64

var (
	errTruncated   = errors.New("truncated msgpack input")
	errTooDeep     = errors.New("msgpack nested too deeply")
	errUnknownTag  = errors.New("unknown msgpack tag")
	errLengthRange = errors.New("msgpack length out of range")
)

type decoder struct {
	buf []byte
	pos int
}

// decodeMsgpack decodes a MsgPack document into plain Go values: nil, bool, int64, float64,
// string, []byte, []any, map[string]any, or Ext.
func decodeMsgpack(data []byte) (any, error) {
	d := &decoder{buf: data}

	return d.decodeValue(0)
}

func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || d.pos+n > len(d.buf) {
		return nil, errTruncated
	}

	b := d.buf[d.pos : d.pos+n]
	d.pos += n

	return b, nil
}

func (d *decoder) byteAt() (byte, error) {
	b, err := d.take(1)
	if err != nil {
		return 0, err
	}

	return b[0], nil
}

// uint reads an n-byte big-endian unsigned integer (MsgPack is big-endian).
func (d *decoder) uint(n int) (uint64, error) {
	b, err := d.take(n)
	if err != nil {
		return 0, err
	}

	var v uint64
	for _, c := range b {
		v = v<<bitsPerByte | uint64(c)
	}

	return v, nil
}

// length reads an n-byte length prefix and narrows it to int, rejecting anything that could not
// possibly be satisfied by the remaining input.
func (d *decoder) length(n int) (int, error) {
	v, err := d.uint(n)
	if err != nil {
		return 0, err
	}

	if v > math.MaxInt32 {
		return 0, errLengthRange
	}

	return int(v), nil
}

func (d *decoder) decodeValue(depth int) (any, error) {
	if depth > maxDecodeDepth {
		return nil, errTooDeep
	}

	c, err := d.byteAt()
	if err != nil {
		return nil, err
	}

	if v, handled, err := d.decodeFixed(c, depth); handled {
		return v, err
	}

	return d.decodeTagged(c, depth)
}

// decodeFixed handles the five formats whose type is encoded in the high bits of the leading byte,
// with the length or value packed into the low bits. handled reports whether c was one of them.
func (d *decoder) decodeFixed(c byte, depth int) (value any, handled bool, err error) {
	switch {
	case c <= maxFixInt:
		return int64(c), true, nil
	case c >= minNegFixInt:
		//nolint:gosec // the range check above makes this conversion exact
		return int64(int8(c)), true, nil
	case c >= minFixMap && c <= maxFixMap:
		v, err := d.decodeMap(int(c&fixMapMask), depth)

		return v, true, err
	case c >= minFixArray && c <= maxFixArray:
		v, err := d.decodeArray(int(c&fixMapMask), depth)

		return v, true, err
	case c >= minFixStr && c <= maxFixStr:
		v, err := d.decodeString(int(c & fixStrMask))

		return v, true, err
	}

	return nil, false, nil
}

// decodeTagged handles the formats that are identified by a whole leading byte rather than by a
// prefix pattern.
//
//nolint:cyclop // a format dispatch table; splitting it further would only obscure it
func (d *decoder) decodeTagged(c byte, depth int) (any, error) {
	switch c {
	case tagNil:
		//nolint:nilnil // a msgpack nil legitimately decodes to a nil value, not an error
		return nil, nil
	case tagFalse:
		return false, nil
	case tagTrue:
		return true, nil
	case tagFloat32, tagFloat64:
		return d.decodeFloat(c)
	}

	switch {
	case c >= tagBin8 && c <= tagBin32:
		return d.decodeBin(1 << (c - tagBin8))
	case c >= tagExt8 && c <= tagExt32:
		return d.decodeExt(1 << (c - tagExt8))
	case c >= tagUint8 && c <= tagUint64:
		return d.decodeUint(1 << (c - tagUint8))
	case c >= tagInt8 && c <= tagInt64:
		return d.decodeInt(1 << (c - tagInt8))
	case c >= tagFixExt1 && c <= tagFixExt16:
		return d.decodeFixExt(1 << (c - tagFixExt1))
	case c >= tagStr8 && c <= tagStr32:
		n, err := d.length(1 << (c - tagStr8))
		if err != nil {
			return nil, err
		}

		return d.decodeString(n)
	case c >= tagArray16 && c <= tagArray32:
		n, err := d.length(wideLenBytes << (c - tagArray16))
		if err != nil {
			return nil, err
		}

		return d.decodeArray(n, depth)
	case c >= tagMap16 && c <= tagMap32:
		n, err := d.length(wideLenBytes << (c - tagMap16))
		if err != nil {
			return nil, err
		}

		return d.decodeMap(n, depth)
	}

	return nil, fmt.Errorf("%w 0x%02x at offset %d", errUnknownTag, c, d.pos-1)
}

func (d *decoder) decodeFloat(c byte) (any, error) {
	if c == tagFloat32 {
		v, err := d.uint(float32Bytes)
		if err != nil {
			return nil, err
		}

		//nolint:gosec // a 4-byte read cannot exceed uint32
		return float64(math.Float32frombits(uint32(v))), nil
	}

	v, err := d.uint(float64Bytes)
	if err != nil {
		return nil, err
	}

	return math.Float64frombits(v), nil
}

func (d *decoder) decodeUint(n int) (any, error) {
	v, err := d.uint(n)
	if err != nil {
		return nil, err
	}

	if v > math.MaxInt64 {
		return nil, errLengthRange
	}

	return int64(v), nil
}

func (d *decoder) decodeInt(n int) (any, error) {
	v, err := d.uint(n)
	if err != nil {
		return nil, err
	}

	// Sign-extend from the n-byte width.
	shift := uint(int64Bits - bitsPerByte*n) //nolint:gosec // n is 1, 2, 4 or 8 by construction

	return int64(v<<shift) >> shift, nil //nolint:gosec // the shift pair keeps this exact
}

func (d *decoder) decodeString(n int) (any, error) {
	b, err := d.take(n)
	if err != nil {
		return nil, err
	}

	return string(b), nil
}

func (d *decoder) decodeBin(lenBytes int) (any, error) {
	n, err := d.length(lenBytes)
	if err != nil {
		return nil, err
	}

	b, err := d.take(n)
	if err != nil {
		return nil, err
	}

	return append([]byte(nil), b...), nil
}

func (d *decoder) decodeArray(n int, depth int) (any, error) {
	// Guard against a bogus length claiming more elements than there are bytes left.
	if n > len(d.buf)-d.pos {
		return nil, errTruncated
	}

	out := make([]any, 0, n)

	for range n {
		v, err := d.decodeValue(depth + 1)
		if err != nil {
			return nil, err
		}

		out = append(out, v)
	}

	return out, nil
}

func (d *decoder) decodeMap(n int, depth int) (any, error) {
	if n > len(d.buf)-d.pos {
		return nil, errTruncated
	}

	out := make(map[string]any, n)

	for range n {
		k, err := d.decodeValue(depth + 1)
		if err != nil {
			return nil, err
		}

		v, err := d.decodeValue(depth + 1)
		if err != nil {
			return nil, err
		}

		// Julia Symbol keys arrive as strings; anything else is stringified so a stray
		// non-string key cannot drop a field silently.
		key, ok := k.(string)
		if !ok {
			key = fmt.Sprint(k)
		}

		out[key] = v
	}

	return out, nil
}

func (d *decoder) decodeFixExt(n int) (any, error) {
	t, err := d.byteAt()
	if err != nil {
		return nil, err
	}

	b, err := d.take(n)
	if err != nil {
		return nil, err
	}

	return makeExt(t, b), nil
}

func (d *decoder) decodeExt(lenBytes int) (any, error) {
	n, err := d.length(lenBytes)
	if err != nil {
		return nil, err
	}

	t, err := d.byteAt()
	if err != nil {
		return nil, err
	}

	b, err := d.take(n)
	if err != nil {
		return nil, err
	}

	return makeExt(t, b), nil
}

func makeExt(t byte, data []byte) any {
	b := append([]byte(nil), data...)
	if t == extUint8 {
		// Vector{UInt8} — image bytes and other raw payloads.
		return b
	}

	return Ext{Type: t, Data: b}
}

// asBytes returns the raw bytes of a decoded value, accepting both the unwrapped []byte form and
// a still-wrapped ext 0x12.
func asBytes(v any) ([]byte, bool) {
	switch b := v.(type) {
	case []byte:
		return b, true
	case Ext:
		if b.Type == extUint8 {
			return b.Data, true
		}
	}

	return nil, false
}
