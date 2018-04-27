// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tiff implements a TIFF image decoder and encoder.
//
// The TIFF specification is at http://partners.adobe.com/public/developer/en/tiff/TIFF6.pdf
package gocog

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"math"
	"strconv"

	"github.com/prl900/scimage"
	"github.com/prl900/scimage/scicolor"
)

// A FormatError reports that the input is not a valid TIFF image.
type FormatError string

func (e FormatError) Error() string {
	return "tiff: invalid format: " + string(e)
}

// An UnsupportedError reports that the input uses a valid but
// unimplemented feature.
type UnsupportedError string

func (e UnsupportedError) Error() string {
	return "tiff: unsupported feature: " + string(e)
}

var errNoPixels = FormatError("not enough pixel data")

type decoder struct {
	r         io.ReaderAt
	byteOrder binary.ByteOrder
	config    image.Config
	mode      imageMode
	sFormat   sampleFormat
	bpp       uint
	features  map[int][]uint
	palette   []color.Color
	noData    float64
	pixScale  []float64
	tiePoint  []float64

	buf   []byte
	off   int    // Current offset in buf.
	v     uint32 // Buffer value for reading with arbitrary bit depths.
	nbits uint   // Remaining number of bits in v.
}

// firstVal returns the first uint of the features entry with the given tag,
// or 0 if the tag does not exist.
func (d *decoder) firstVal(tag int) uint {
	f := d.features[tag]
	if len(f) == 0 {
		return 0
	}
	return f[0]
}

// ifdUint decodes the IFD entry in p, which must be of the Byte, Short
// or Long type, and returns the decoded uint values.
func (d *decoder) ifdUint(p []byte) (u []uint, err error) {
	var raw []byte
	if len(p) < ifdLen {
		return nil, FormatError("bad IFD entry")
	}

	datatype := d.byteOrder.Uint16(p[2:4])
	if dt := int(datatype); dt <= 0 || dt >= len(lengths) {
		return nil, UnsupportedError("IFD entry datatype")
	}

	count := d.byteOrder.Uint32(p[4:8])
	if count > math.MaxInt32/lengths[datatype] {
		return nil, FormatError("IFD data too large")
	}
	if datalen := lengths[datatype] * count; datalen > 4 {
		// The IFD contains a pointer to the real value.
		raw = make([]byte, datalen)
		_, err = d.r.ReadAt(raw, int64(d.byteOrder.Uint32(p[8:12])))
	} else {
		raw = p[8 : 8+datalen]
	}
	if err != nil {
		return nil, err
	}

	u = make([]uint, count)
	switch datatype {
	case dtByte, dtASCII:
		for i := uint32(0); i < count; i++ {
			u[i] = uint(raw[i])
		}
	case dtShort:
		for i := uint32(0); i < count; i++ {
			u[i] = uint(d.byteOrder.Uint16(raw[2*i : 2*(i+1)]))
		}
	case dtLong:
		for i := uint32(0); i < count; i++ {
			u[i] = uint(d.byteOrder.Uint32(raw[4*i : 4*(i+1)]))
		}
	case dtFloat64:
		for i := uint32(0); i < count; i++ {
			u[i] = uint(d.byteOrder.Uint64(raw[8*i : 8*(i+1)]))
		}
	default:
		return nil, UnsupportedError("data type")
	}
	return u, nil
}

// parseIFD decides whether the IFD entry in p is "interesting" and
// stows away the data in the decoder. It returns the tag number of the
// entry and an error, if any.
func (d *decoder) parseIFD(p []byte) (int, error) {
	tag := d.byteOrder.Uint16(p[0:2])

	switch tag {
	case tBitsPerSample,
		tExtraSamples,
		tPhotometricInterpretation,
		tCompression,
		tPredictor,
		tStripOffsets,
		tStripByteCounts,
		tRowsPerStrip,
		tTileWidth,
		tTileLength,
		tTileOffsets,
		tTileByteCounts,
		tImageLength,
		tImageWidth:
		val, err := d.ifdUint(p)
		if err != nil {
			return 0, err
		}
		d.features[int(tag)] = val
	case tOrientation,
		tXResolution,
		tYResolution,
		tXPosition,
		tYPosition,
		tResolutionUnit:
		d.ifdUint(p)

	case tModelTiepoint:
		val, err := d.ifdUint(p)
		if err != nil {
			return 0, err
		}

		d.tiePoint = make([]float64, len(val))
		for i, v := range val {
			d.tiePoint[i] = math.Float64frombits(uint64(v))
		}

	//* TODO: Need to find Projection info
	case tGeoKeyDirectory:
		fmt.Println("BBB")
		val, err := d.ifdUint(p)
		if err != nil {
			return 0, err
		}
		if len(val)%4 != 0 {
			return 0, err
		}
		fmt.Println("AAA", len(val))
		for i := 0; i < (len(val) / 4); i++ {
			fmt.Printf("KeyID: %d, TIFFTagLocation: %d, Count: %d, Value_Offset: %d\n", val[i*4], val[(i*4)+1], val[(i*4)+2], val[(i*4)+3])
		}

	case tModelPixelScale:
		val, err := d.ifdUint(p)
		if err != nil {
			return 0, err
		}

		d.pixScale = make([]float64, len(val))
		for i, v := range val {
			d.pixScale[i] = math.Float64frombits(uint64(v))
		}

	case tGDALNoData:
		val, err := d.ifdUint(p)
		if err != nil {
			return 0, err
		}
		str := make([]byte, len(val))
		for i, v := range val {
			str[i] = byte(v)
		}
		f, err := strconv.ParseFloat(string(bytes.Trim(str, "\x00")), 64)
		if err != nil {
			return 0, err
		}
		d.noData = f

	case tColorMap:
		val, err := d.ifdUint(p)
		if err != nil {
			return 0, err
		}
		numcolors := len(val) / 3
		if len(val)%3 != 0 || numcolors <= 0 || numcolors > 256 {
			return 0, FormatError("bad ColorMap length")
		}
		d.palette = make([]color.Color, numcolors)
		for i := 0; i < numcolors; i++ {
			d.palette[i] = color.RGBA64{
				uint16(val[i]),
				uint16(val[i+numcolors]),
				uint16(val[i+2*numcolors]),
				0xffff,
			}
		}
	case tSampleFormat:
		val, err := d.ifdUint(p)
		if err != nil {
			return 0, err
		}
		d.sFormat = sampleFormat(val[0])
	}
	return int(tag), nil
}

// readBits reads n bits from the internal buffer starting at the current offset.
func (d *decoder) readBits(n uint) (v uint32, ok bool) {
	for d.nbits < n {
		d.v <<= 8
		if d.off >= len(d.buf) {
			return 0, false
		}
		d.v |= uint32(d.buf[d.off])
		d.off++
		d.nbits += 8
	}
	d.nbits -= n
	rv := d.v >> d.nbits
	d.v &^= rv << d.nbits
	return rv, true
}

// flushBits discards the unread bits in the buffer used by readBits.
// It is used at the end of a line.
func (d *decoder) flushBits() {
	d.v = 0
	d.nbits = 0
}

// minInt returns the smaller of x or y.
func minInt(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

// decode decodes the raw data of an image.
// It reads from d.buf and writes the strip or tile into dst.
func (d *decoder) decode(dst image.Image, xmin, ymin, xmax, ymax int) error {
	d.off = 0

	// Apply horizontal predictor if necessary.
	// In this case, p contains the color difference to the preceding pixel.
	// See page 64-65 of the spec.
	if d.firstVal(tPredictor) == prHorizontal {
		switch d.bpp {
		case 16:
			var off int
			n := 2 * len(d.features[tBitsPerSample]) // bytes per sample times samples per pixel
			for y := ymin; y < ymax; y++ {
				off += n
				for x := 0; x < (xmax-xmin-1)*n; x += 2 {
					if off+2 > len(d.buf) {
						return errNoPixels
					}
					v0 := d.byteOrder.Uint16(d.buf[off-n : off-n+2])
					v1 := d.byteOrder.Uint16(d.buf[off : off+2])
					d.byteOrder.PutUint16(d.buf[off:off+2], v1+v0)
					off += 2
				}
			}
		case 8:
			var off int
			n := 1 * len(d.features[tBitsPerSample]) // bytes per sample times samples per pixel
			for y := ymin; y < ymax; y++ {
				off += n
				for x := 0; x < (xmax-xmin-1)*n; x++ {
					if off >= len(d.buf) {
						return errNoPixels
					}
					d.buf[off] += d.buf[off-n]
					off++
				}
			}
		case 1:
			return UnsupportedError("horizontal predictor with 1 BitsPerSample")
		}
	}

	rMaxX := minInt(xmax, dst.Bounds().Max.X)
	rMaxY := minInt(ymax, dst.Bounds().Max.Y)

	switch d.mode {
	case mGray, mGrayInvert:
		switch d.sFormat {
		case uintSample:
			if d.bpp == 16 {
				img := dst.(*scimage.GrayU16)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if d.off+2 > len(d.buf) {
							return errNoPixels
						}
						v := uint16(d.buf[d.off+0])<<8 | uint16(d.buf[d.off+1])
						d.off += 2
						if d.mode == mGrayInvert {
							v = 0xffff - v
						}
						img.SetGrayU16(x, y, scicolor.GrayU16{v, img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						d.off += 2 * (xmax - img.Bounds().Max.X)
					}
				}
			} else {
				img := dst.(*scimage.GrayU8)
				max := uint32((1 << d.bpp) - 1)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						v, ok := d.readBits(d.bpp)
						if !ok {
							return errNoPixels
						}
						v = v * 0xff / max
						if d.mode == mGrayInvert {
							v = 0xff - v
						}
						img.SetGrayU8(x, y, scicolor.GrayU8{uint8(v), img.Min, img.Max})
					}
					d.flushBits()
				}
			}
		case sintSample:
			if d.bpp == 16 {
				img := dst.(*scimage.GrayS16)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if d.off+2 > len(d.buf) {
							return errNoPixels
						}
						v := int16(d.buf[d.off+0])<<8 | int16(d.buf[d.off+1])
						d.off += 2
						//TODO Invert a signed int?
						/*
							if d.mode == mGrayInvert {
								v = 0xffff - v
							}*/
						img.SetGrayS16(x, y, scicolor.GrayS16{v, img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						d.off += 2 * (xmax - img.Bounds().Max.X)
					}
				}
			} else {
				img := dst.(*scimage.GrayS8)
				max := uint32((1 << d.bpp) - 1)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						v, ok := d.readBits(d.bpp)
						if !ok {
							return errNoPixels
						}
						v = v * 0xff / max
						//TODO Invert a signed int?
						/*if d.mode == mGrayInvert {
							v = 0xff - v
						}*/
						img.SetGrayS8(x, y, scicolor.GrayS8{int8(v), img.Min, img.Max})
					}
					d.flushBits()
				}
			}
		}

	default:
		return FormatError("malformed header")

	}

	return nil
}

// TODO: Does cog need to support stripped files?
// TODO: stripped files are not implemented for the moment
type ImgDesc struct {
	NewSubfileType     uint32
	ImageWidth         uint32
	ImageHeight        uint32
	BitsPerSample      uint16
	Compression        uint16
	PhotometricInterpr uint16
	SamplesPerPixel    uint16
	TileWidth          uint32
	TileHeight         uint32
	SampleFormat       uint16
	TileOffsets        []uint32
	TileByteCounts     []uint32
}

// readBits reads n bits from the internal buffer starting at the current offset.
func (d *simpleDec) readBits(n uint) (v uint32, ok bool) {
	for d.nbits < n {
		d.v <<= 8
		if d.off >= len(d.buf) {
			return 0, false
		}
		d.v |= uint32(d.buf[d.off])
		d.off++
		d.nbits += 8
	}
	d.nbits -= n
	rv := d.v >> d.nbits
	d.v &^= rv << d.nbits
	return rv, true
}

// flushBits discards the unread bits in the buffer used by readBits.
// It is used at the end of a line.
func (d *simpleDec) flushBits() {
	d.v = 0
	d.nbits = 0
}
// parseIFD decides whether the IFD entry in p is "interesting" and
// stows away the data in the decoder. It returns the tag number of the
// entry and an error, if any.
func (d *simpleDec) ParseIFD(ifdOffset int64) (int64, error) {

	p := make([]byte, 8)
	if _, err := d.ra.ReadAt(p[0:2], ifdOffset); err != nil {
		return 0, FormatError("error reading IFD")
	}
	numItems := int(d.bo.Uint16(p[0:2]))

	ifd := make([]byte, ifdLen*numItems)
	if _, err := d.ra.ReadAt(ifd, ifdOffset+2); err != nil {
		return 0, FormatError("error reading IFD")
	}
	imgDesc := ImgDesc{}
	for i := 0; i < len(ifd); i += ifdLen {

		tag := d.bo.Uint16(ifd[i : i+2])
		datatype := d.bo.Uint16(ifd[i+2 : i+4])
		count := d.bo.Uint32(ifd[i+4 : i+8])

		switch tag {
		case cNewSubfileType:
			if datatype != dtLong || count != 1 {
				fmt.Println(datatype, count)
				return 0, FormatError("unexpected value found on IFD")
			}
			imgDesc.NewSubfileType = d.bo.Uint32(ifd[i+8 : i+12])
		case cImageWidth:
			if count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			switch datatype {
			case dtShort:
				imgDesc.ImageWidth = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.ImageWidth = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError("unexpected value found on IFD")
			}
		case cImageLength:
			if count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			switch datatype {
			case dtShort:
				imgDesc.ImageHeight = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.ImageHeight = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError("unexpected value found on IFD")
			}
		case cBitsPerSample:
			if datatype != dtShort || count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			imgDesc.BitsPerSample = d.bo.Uint16(ifd[i+8 : i+10])
		case cCompression:
			if datatype != dtShort || count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			imgDesc.Compression = d.bo.Uint16(ifd[i+8 : i+10])
		case cPhotometricInterpr:
			if datatype != dtShort || count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			imgDesc.PhotometricInterpr = d.bo.Uint16(ifd[i+8 : i+10])
		case cSamplesPerPixel:
			if datatype != dtShort || count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			imgDesc.SamplesPerPixel = d.bo.Uint16(ifd[i+8 : i+10])
		case cSampleFormat:
			if datatype != dtShort || count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			imgDesc.SampleFormat = d.bo.Uint16(ifd[i+8 : i+10])
		case cTileWidth:
			if count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			switch datatype {
			case dtShort:
				imgDesc.TileWidth = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.TileWidth = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError("unexpected value found on IFD")
			}
		case cTileLength:
			if count != 1 {
				return 0, FormatError("unexpected value found on IFD")
			}
			switch datatype {
			case dtShort:
				imgDesc.TileHeight = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.TileHeight = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError("unexpected value found on IFD")
			}
		case cTileOffsets, cTileByteCounts:
			if datatype != dtLong {
				return 0, FormatError("unexpected value found on IFD")
			}

			var raw []byte
			if datalen := int(lengths[datatype] * count); datalen > 4 {
				// The IFD contains a pointer to the real value.
				raw = make([]byte, datalen)
				d.ra.ReadAt(raw, int64(d.bo.Uint32(ifd[i+8:i+12])))
			} else {
				raw = ifd[i+8 : i+8+datalen]
			}
			data := make([]uint32, count)
			for i := uint32(0); i < count; i++ {
				data[i] = d.bo.Uint32(raw[4*i : 4*(i+1)])
			}
			if tag == cTileOffsets {
				imgDesc.TileOffsets = data
			} else {
				imgDesc.TileByteCounts = data
			}
		}
	}
	d.nameit = append(d.nameit, imgDesc)

	nextIFDOffset := ifdOffset + int64(2) + int64(numItems*12)
	if _, err := d.ra.ReadAt(p[0:4], nextIFDOffset); err != nil {
		return 0, FormatError("error reading IFD")
	}
	ifdOffset = int64(d.bo.Uint32(p[:4]))

	return ifdOffset, nil
}

type simpleDec struct {
	buf    []byte
	ra     io.ReaderAt
	bo     binary.ByteOrder
	nameit []ImgDesc

	off int
	v     uint32 // Buffer value for reading with arbitrary bit depths.
	nbits uint   // Remaining number of bits in v.
}

func newSimpleDec(r io.Reader) (simpleDec, error) {
	ra := newReaderAt(r)

	p := make([]byte, 8)
	if _, err := ra.ReadAt(p, 0); err != nil {
		return simpleDec{}, FormatError("malformed header")
	}
	switch string(p[0:4]) {
	case leHeader:
		return simpleDec{nil, ra, binary.LittleEndian, []ImgDesc{}, 0, 0, 0}, nil
	case beHeader:
		return simpleDec{nil, ra, binary.BigEndian, []ImgDesc{}, 0, 0, 0}, nil
	}

	return simpleDec{}, FormatError("malformed header")
}

func (d *simpleDec) ReadIFD() {

	p := make([]byte, 4)
	if _, err := d.ra.ReadAt(p, 4); err != nil {
		return
	}
	ifdOffset := int64(d.bo.Uint32(p[0:4]))

	for ifdOffset != 0 {
		ifdOffset, _ = d.ParseIFD(ifdOffset)

	}
}

// decode decodes the raw data of an image.
// It reads from d.buf and writes the strip or tile into dst.
func (d *simpleDec) decode(dst image.Image, level, xmin, ymin, xmax, ymax int) error {

	cfg := d.nameit[level]
	d.off = 0

	rMaxX := minInt(xmax, dst.Bounds().Max.X)
	rMaxY := minInt(ymax, dst.Bounds().Max.Y)

	if cfg.SamplesPerPixel != 1 {

	}

	switch cfg.PhotometricInterpr {
	case pBlackIsZero:
		switch sampleFormat(cfg.SampleFormat) {
		case uintSample:
			switch cfg.BitsPerSample {
			case 8:
				img := dst.(*scimage.GrayU8)
				max := uint32((1 << cfg.BitsPerSample) - 1)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						v, ok := d.readBits(uint(cfg.BitsPerSample))
						if !ok {
							return errNoPixels
						}
						v = v * 0xff / max
						img.SetGrayU8(x, y, scicolor.GrayU8{uint8(v), img.Min, img.Max})
					}
					d.flushBits()
				}
			case 16:
				img := dst.(*scimage.GrayU16)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if d.off+2 > len(d.buf) {
							return errNoPixels
						}
						v := uint16(d.buf[d.off+0])<<8 | uint16(d.buf[d.off+1])
						d.off += 2
						img.SetGrayU16(x, y, scicolor.GrayU16{v, img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						d.off += 2 * (xmax - img.Bounds().Max.X)
					}
				}
			}
		case sintSample:
			switch cfg.BitsPerSample {
			case 8:
				img := dst.(*scimage.GrayS8)
				max := uint32((1 << cfg.BitsPerSample) - 1)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						v, ok := d.readBits(uint(cfg.BitsPerSample))
						if !ok {
							return errNoPixels
						}
						v = v * 0xff / max
						img.SetGrayS8(x, y, scicolor.GrayS8{int8(v), img.Min, img.Max})
					}
					d.flushBits()
				}
			case 16:
				img := dst.(*scimage.GrayS16)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if d.off+2 > len(d.buf) {
							return errNoPixels
						}
						v := int16(d.buf[d.off+0])<<8 | int16(d.buf[d.off+1])
						d.off += 2
						img.SetGrayS16(x, y, scicolor.GrayS16{v, img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						d.off += 2 * (xmax - img.Bounds().Max.X)
					}
				}
			}
		}

	default:
		return FormatError("malformed header")

	}

	return nil
}
func SimpleDecode(r io.Reader, level int) (img image.Image, err error) {
	d, err := newSimpleDec(r)
	if err != nil {
		return
	}
	d.ReadIFD()

	fmt.Println("AAAA", d.nameit)

	cfg := d.nameit[level]

	blockPadding := false
	blocksAcross := 1
	blocksDown := 1

	if cfg.ImageWidth == 0 || cfg.ImageHeight == 0 {
		return nil, FormatError("image data type not implemented")

	}

	if cfg.TileWidth != 0 {
		blockPadding = true
		blocksAcross = int((cfg.ImageWidth + cfg.TileWidth - 1) / cfg.TileWidth)
		if cfg.TileHeight != 0 {
			blocksDown = int((cfg.ImageHeight + cfg.TileHeight - 1) / cfg.TileHeight)
		}
	}

	// Check if we have the right number of strips/tiles, offsets and counts.
	if n := blocksAcross * blocksDown; len(cfg.TileOffsets) < n || len(cfg.TileByteCounts) < n {
		return nil, FormatError("inconsistent header")
	}

	switch cfg.BitsPerSample {
	case 0:
		return nil, FormatError("BitsPerSample must not be 0")
	case 1, 8, 16:
		// Nothing to do, these are accepted by this implementation.
	default:
		return nil, UnsupportedError(fmt.Sprintf("BitsPerSample of %v", cfg.BitsPerSample))
	}

	imgRect := image.Rect(0, 0, int(cfg.ImageWidth), int(cfg.ImageHeight))
	switch cfg.PhotometricInterpr {
	case pBlackIsZero:
		switch sampleFormat(cfg.SampleFormat) {
		case uintSample:
			switch cfg.BitsPerSample {
			case 8:
				img = scimage.NewGrayU8(imgRect, 0, 255)
			case 16:
				img = scimage.NewGrayU16(imgRect, 0, 65535)
			default:
				return nil, FormatError("image data type not implemented")
			}
		case sintSample:
			switch cfg.BitsPerSample {
			case 8:
				img = scimage.NewGrayS8(imgRect, -128, 127)
			case 16:
				img = scimage.NewGrayS16(imgRect, 0, 32767)
			default:
				return nil, FormatError("image data type not implemented")
			}
		default:
			return nil, FormatError("image data type not implemented")
		}
	default:
		return nil, FormatError("color model not implemented")
	}

	for i := 0; i < blocksAcross; i++ {
		blkW := int(cfg.TileWidth)
		if !blockPadding && i == blocksAcross-1 && cfg.ImageWidth%cfg.TileWidth != 0 {
			blkW = int(cfg.ImageWidth % cfg.TileWidth)
		}
		for j := 0; j < blocksDown; j++ {
			blkH := int(cfg.TileHeight)
			if !blockPadding && j == blocksDown-1 && cfg.ImageHeight%cfg.TileHeight != 0 {
				blkH = int(cfg.ImageHeight % cfg.TileHeight)
			}
			offset := int64(cfg.TileOffsets[j*blocksAcross+i])
			n := int64(cfg.TileByteCounts[j*blocksAcross+i])
			switch cfg.Compression {

			// According to the spec, Compression does not have a default value,
			// but some tools interpret a missing Compression value as none so we do
			// the same.
			case cNone, 0:
				if b, ok := d.ra.(*buffer); ok {
					d.buf, err = b.Slice(int(offset), int(n))
				} else {
					d.buf = make([]byte, n)
					_, err = d.ra.ReadAt(d.buf, offset)
				}
			case cDeflate, cDeflateOld:
				var r io.ReadCloser
				r, err = zlib.NewReader(io.NewSectionReader(d.ra, offset, n))
				if err != nil {
					return nil, err
				}
				d.buf, err = ioutil.ReadAll(r)
				r.Close()
			case cPackBits:
				d.buf, err = unpackBits(io.NewSectionReader(d.ra, offset, n))
			default:
				err = UnsupportedError(fmt.Sprintf("compression value %d", cfg.Compression))
			}
			if err != nil {
				return nil, err
			}

			xmin := i * int(cfg.TileWidth)
			ymin := j * int(cfg.TileHeight)
			xmax := xmin + blkW
			ymax := ymin + blkH

			err = d.decode(img, level, xmin, ymin, xmax, ymax)
			if err != nil {
				return nil, err
			}
		}
	}
	return
}

func newDecoder(r io.Reader) (*decoder, error) {
	d := &decoder{
		r:        newReaderAt(r),
		features: make(map[int][]uint),
	}

	p := make([]byte, 8)
	if _, err := d.r.ReadAt(p, 0); err != nil {
		return nil, err
	}
	switch string(p[0:4]) {
	case leHeader:
		d.byteOrder = binary.LittleEndian
	case beHeader:
		d.byteOrder = binary.BigEndian
	default:
		return nil, FormatError("malformed header")
	}

	fmt.Println("IFD Entries =", p)

	ifdOffset := int64(d.byteOrder.Uint32(p[4:8]))

	// The first two bytes contain the number of entries (12 bytes each).
	if _, err := d.r.ReadAt(p[0:2], ifdOffset); err != nil {
		return nil, err
	}
	numItems := int(d.byteOrder.Uint16(p[0:2]))

	fmt.Println("IFD Entries =", numItems)
	nextIFDOffset := ifdOffset + int64(2) + int64(numItems*12)
	if _, err := d.r.ReadAt(p[0:4], nextIFDOffset); err != nil {
		return nil, err
	}
	nextIFD := int64(d.byteOrder.Uint32(p[:4]))
	fmt.Println("AAAAAA", nextIFD)

	// All IFD entries are read in one chunk.
	p = make([]byte, ifdLen*numItems)
	if _, err := d.r.ReadAt(p, ifdOffset+2); err != nil {
		return nil, err
	}

	prevTag := -1
	for i := 0; i < len(p); i += ifdLen {
		tag, err := d.parseIFD(p[i : i+ifdLen])
		if err != nil {
			return nil, err
		}
		if tag <= prevTag {
			return nil, FormatError("tags are not sorted in ascending order")
		}
		prevTag = tag
	}

	d.config.Width = int(d.firstVal(tImageWidth))
	d.config.Height = int(d.firstVal(tImageLength))

	if _, ok := d.features[tBitsPerSample]; !ok {
		return nil, FormatError("BitsPerSample tag missing")
	}
	d.bpp = d.firstVal(tBitsPerSample)
	switch d.bpp {
	case 0:
		return nil, FormatError("BitsPerSample must not be 0")
	case 1, 8, 16:
		// Nothing to do, these are accepted by this implementation.
	default:
		return nil, UnsupportedError(fmt.Sprintf("BitsPerSample of %v", d.bpp))
	}

	switch d.firstVal(tPhotometricInterpretation) {
	case pBlackIsZero:
		d.mode = mGray
		if d.bpp == 16 {
			d.config.ColorModel = scicolor.GrayU16Model{Min: 0, Max: 10000}
		} else {
			d.config.ColorModel = scicolor.GrayU8Model{Min: 0, Max: 255}
		}
	default:
		return nil, UnsupportedError("color model")
	}

	return d, nil
}

// DecodeConfig returns the color model and dimensions of a TIFF image without
// decoding the entire image.
func DecodeConfig(r io.Reader) (image.Config, error) {
	d, err := newDecoder(r)
	if err != nil {
		return image.Config{}, err
	}
	return d.config, nil
}

// Decode reads a TIFF image from r and returns it as an image.Image.
// The type of Image returned depends on the contents of the TIFF.
func Decode(r io.Reader) (img image.Image, err error) {
	d, err := newDecoder(r)
	if err != nil {
		return
	}

	fmt.Println("Debugging:", d)
	fmt.Println("Debugging:", d.config.Width, d.config.Height)
	fmt.Println("Debugging:", int(d.firstVal(tTileWidth)), int(d.firstVal(tTileLength)))
	fmt.Println("Debugging:", d.features[tTileOffsets])
	fmt.Println("Debugging:", d.features[tTileByteCounts])

	return nil, fmt.Errorf("Debugging")

	blockPadding := false
	blockWidth := d.config.Width
	blockHeight := d.config.Height
	blocksAcross := 1
	blocksDown := 1

	if d.config.Width == 0 {
		blocksAcross = 0
	}
	if d.config.Height == 0 {
		blocksDown = 0
	}

	var blockOffsets, blockCounts []uint

	if int(d.firstVal(tTileWidth)) != 0 {
		blockPadding = true

		blockWidth = int(d.firstVal(tTileWidth))
		blockHeight = int(d.firstVal(tTileLength))

		if blockWidth != 0 {
			blocksAcross = (d.config.Width + blockWidth - 1) / blockWidth
		}
		if blockHeight != 0 {
			blocksDown = (d.config.Height + blockHeight - 1) / blockHeight
		}

		blockCounts = d.features[tTileByteCounts]
		blockOffsets = d.features[tTileOffsets]
	} else {
		if int(d.firstVal(tRowsPerStrip)) != 0 {
			blockHeight = int(d.firstVal(tRowsPerStrip))
		}

		if blockHeight != 0 {
			blocksDown = (d.config.Height + blockHeight - 1) / blockHeight
		}

		blockOffsets = d.features[tStripOffsets]
		blockCounts = d.features[tStripByteCounts]
	}

	// Check if we have the right number of strips/tiles, offsets and counts.
	if n := blocksAcross * blocksDown; len(blockOffsets) < n || len(blockCounts) < n {
		return nil, FormatError("inconsistent header")
	}

	imgRect := image.Rect(0, 0, d.config.Width, d.config.Height)
	switch d.mode {
	case mGray, mGrayInvert:
		switch d.sFormat {
		case uintSample:
			if d.bpp == 16 {
				// TODO: This is a hack to test new geospatial types that implement the Image interface
				//img = &scimage.NewGrayU16(imgRect), "", []float64{d.tiePoint[3], d.pixScale[0], 0, d.tiePoint[4], 0, -1 * d.pixScale[1]}, d.noData}
				img = scimage.NewGrayU16(imgRect, 0, 65535)
			} else {
				img = scimage.NewGrayU8(imgRect, 0, 255)
			}
		case sintSample:
			if d.bpp == 16 {
				//img = scimage.NewGrayS16(imgRect, -32768, 32767)
				img = scimage.NewGrayS16(imgRect, 0, 32767)
			} else {
				img = scimage.NewGrayS8(imgRect, -128, 127)
			}
		default:
			return nil, FormatError("image data type not implemented")
		}
	default:
		return nil, FormatError("color model not implemented")
	}

	for i := 0; i < blocksAcross; i++ {
		blkW := blockWidth
		if !blockPadding && i == blocksAcross-1 && d.config.Width%blockWidth != 0 {
			blkW = d.config.Width % blockWidth
		}
		for j := 0; j < blocksDown; j++ {
			blkH := blockHeight
			if !blockPadding && j == blocksDown-1 && d.config.Height%blockHeight != 0 {
				blkH = d.config.Height % blockHeight
			}
			offset := int64(blockOffsets[j*blocksAcross+i])
			n := int64(blockCounts[j*blocksAcross+i])
			switch d.firstVal(tCompression) {

			// According to the spec, Compression does not have a default value,
			// but some tools interpret a missing Compression value as none so we do
			// the same.
			case cNone, 0:
				if b, ok := d.r.(*buffer); ok {
					d.buf, err = b.Slice(int(offset), int(n))
				} else {
					d.buf = make([]byte, n)
					_, err = d.r.ReadAt(d.buf, offset)
				}
			case cDeflate, cDeflateOld:
				var r io.ReadCloser
				r, err = zlib.NewReader(io.NewSectionReader(d.r, offset, n))
				if err != nil {
					return nil, err
				}
				d.buf, err = ioutil.ReadAll(r)
				r.Close()
			case cPackBits:
				d.buf, err = unpackBits(io.NewSectionReader(d.r, offset, n))
			default:
				err = UnsupportedError(fmt.Sprintf("compression value %d", d.firstVal(tCompression)))
			}
			if err != nil {
				return nil, err
			}

			xmin := i * blockWidth
			ymin := j * blockHeight
			xmax := xmin + blkW
			ymax := ymin + blkH
			err = d.decode(img, xmin, ymin, xmax, ymax)
			if err != nil {
				return nil, err
			}
		}
	}
	return
}

func init() {
	image.RegisterFormat("tiff", leHeader, Decode, DecodeConfig)
	image.RegisterFormat("tiff", beHeader, Decode, DecodeConfig)
}
