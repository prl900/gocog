// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tiff implements a TIFF image decoder and encoder.
//
// The TIFF specification is at http://partners.adobe.com/public/developer/en/tiff/TIFF6.pdf
package gocog

import (
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"io/ioutil"

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

// minInt returns the smaller of x or y.
func minInt(a, b int) int {
	if a <= b {
		return a
	}
	return b
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

type decoder struct {
	buf    []byte
	ra     io.ReaderAt
	bo     binary.ByteOrder
	nameit []ImgDesc

	//off int
}

// parseIFD decides whether the IFD entry in p is "interesting" and
// stows away the data in the decoder. It returns the tag number of the
// entry and an error, if any.
func (d *decoder) ParseIFD(ifdOffset int64) (int64, error) {

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

func newDecoder(r io.Reader) (decoder, error) {
	ra := newReaderAt(r)

	p := make([]byte, 8)
	if _, err := ra.ReadAt(p, 0); err != nil {
		return decoder{}, FormatError("malformed header")
	}
	switch string(p[0:4]) {
	case leHeader:
		return decoder{nil, ra, binary.LittleEndian, []ImgDesc{}}, nil
	case beHeader:
		return decoder{nil, ra, binary.BigEndian, []ImgDesc{}}, nil
	}

	return decoder{}, FormatError("malformed header")
}

func (d *decoder) ReadIFD() {

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
func (d *decoder) decode(dst image.Image, level, xmin, ymin, xmax, ymax int) error {
	cfg := d.nameit[level]
	off := 0

	rMaxX := minInt(xmax, dst.Bounds().Max.X)
	rMaxY := minInt(ymax, dst.Bounds().Max.Y)

	if cfg.SamplesPerPixel != 1 {
		return FormatError("image data type not implemented")
	}

	switch cfg.PhotometricInterpr {
	case pBlackIsZero:
		switch sampleFormat(cfg.SampleFormat) {
		case uintSample:
			switch cfg.BitsPerSample {
			case 8:
				img := dst.(*scimage.GrayU8)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if off+1 > len(d.buf) {
							return errNoPixels
						}
						v := uint8(d.buf[off+0])
						off++
						img.SetGrayU8(x, y, scicolor.GrayU8{uint8(v), img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						off += xmax - img.Bounds().Max.X
					}
				}
			case 16:
				img := dst.(*scimage.GrayU16)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if off+2 > len(d.buf) {
							return errNoPixels
						}
						v := uint16(d.buf[off+0])<<8 | uint16(d.buf[off+1])
						off += 2
						img.SetGrayU16(x, y, scicolor.GrayU16{v, img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						off += 2 * (xmax - img.Bounds().Max.X)
					}
				}
			}
		case sintSample:
			switch cfg.BitsPerSample {
			case 8:
				img := dst.(*scimage.GrayS8)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if off+1 > len(d.buf) {
							return errNoPixels
						}
						v := int8(d.buf[off+0])
						off++
						img.SetGrayS8(x, y, scicolor.GrayS8{int8(v), img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						off += xmax - img.Bounds().Max.X
					}
				}
			case 16:
				img := dst.(*scimage.GrayS16)
				for y := ymin; y < rMaxY; y++ {
					for x := xmin; x < rMaxX; x++ {
						if off+2 > len(d.buf) {
							return errNoPixels
						}
						v := int16(d.buf[off+0])<<8 | int16(d.buf[off+1])
						off += 2
						img.SetGrayS16(x, y, scicolor.GrayS16{v, img.Min, img.Max})
					}
					if rMaxX == img.Bounds().Max.X {
						off += 2 * (xmax - img.Bounds().Max.X)
					}
				}
			}
		}

	default:
		return FormatError("malformed header")

	}

	return nil
}

func Decode(r io.Reader, level int) (img image.Image, err error) {
	d, err := newDecoder(r)
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

/*
// DecodeConfig returns the color model and dimensions of a TIFF image without
// decoding the entire image.
func DecodeConfig(r io.Reader) (image.Config, error) {
	d, err := newDecoder(r)
	if err != nil {
		return image.Config{}, err
	}
	return d.config, nil
}

func init() {
	image.RegisterFormat("cog", leHeader, Decode, DecodeConfig)
	image.RegisterFormat("cog", beHeader, Decode, DecodeConfig)
}
*/
