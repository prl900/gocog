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
	"log"
	"image"
	"image/color"
	"io"
	"io/ioutil"

	"github.com/terrascope/gocog/lzw"
	"github.com/terrascope/scimage"
	"github.com/terrascope/scimage/scicolor"
	"math"
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
	TileWidth          uint32
	TileHeight         uint32
	PhotometricInterpr uint16
	Predictor          uint16
	Compression        uint16
	SamplesPerPixel    uint16
	BitsPerSample      []uint16
	SampleFormat       []uint16
	TileOffsets        []uint32
	TileByteCounts     []uint32
}

type decoder struct {
	buf []byte
	ra  io.ReaderAt
	bo  binary.ByteOrder
	dsc []ImgDesc
}

func newDecoder(r io.Reader) (decoder, error) {
	ra := newReaderAt(r)
	p := make([]byte, 8)
	if _, err := ra.ReadAt(p, 0); err != nil {
		return decoder{}, FormatError("malformed header 1")
	}
	switch string(p[0:4]) {
	case leHeader:
		return decoder{nil, ra, binary.LittleEndian, []ImgDesc{}}, nil
	case beHeader:
		return decoder{nil, ra, binary.BigEndian, []ImgDesc{}}, nil
	}

	return decoder{}, FormatError("malformed header 2")
}

// parseIFD decides whether the IFD entry in p is "interesting" and
// stows away the data in the decoder. It returns the tag number of the
// entry and an error, if any.
func (d *decoder) parseIFD(ifdOffset int64) (int64, error) {

	p := make([]byte, 8)
	if _, err := d.ra.ReadAt(p[0:2], ifdOffset); err != nil {
		return 0, FormatError("error reading IFD")
	}
	numItems := int(d.bo.Uint16(p[0:2]))

	ifd := make([]byte, ifdLen*numItems)
	if _, err := d.ra.ReadAt(ifd, ifdOffset+2); err != nil {
		return 0, FormatError("error reading IFD")
	}
	var kEntries []KeyEntry
	var dParams []float64
	var aParams string
	var tiePoint []float64
	var pixelScale []float64

	imgDesc := ImgDesc{SampleFormat:[]uint16{1}, Predictor:1}
	nonCaptTags := []uint16{}

	for i := 0; i < len(ifd); i += ifdLen {

		tag := d.bo.Uint16(ifd[i : i+2])
		datatype := d.bo.Uint16(ifd[i+2 : i+4])
		count := d.bo.Uint32(ifd[i+4 : i+8])

		switch tag {
		case cNewSubfileType:
			if datatype != dtLong || count != 1 {
				return 0, FormatError(fmt.Sprintf("NewSubfileType type: %v not recognised", datatype))
			}
			imgDesc.NewSubfileType = d.bo.Uint32(ifd[i+8 : i+12])
		case cImageWidth:
			if count != 1 {
				return 0, FormatError(fmt.Sprintf("ImageWidth count: %d not recognised", count))
			}
			switch datatype {
			case dtShort:
				imgDesc.ImageWidth = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.ImageWidth = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError(fmt.Sprintf("ImageWidth type: %d not recognised", datatype))
			}
		case cImageLength:
			if count != 1 {
				return 0, FormatError(fmt.Sprintf("ImageLength count: %d not recognised", count))
			}
			switch datatype {
			case dtShort:
				imgDesc.ImageHeight = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.ImageHeight = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError(fmt.Sprintf("ImageLength type: %v not recognised", datatype))
			}
		case cBitsPerSample:
			if datatype != dtShort {
				return 0, FormatError(fmt.Sprintf("BitsPerSample type: %v not recognised", datatype))
			}
			imgDesc.BitsPerSample = []uint16{d.bo.Uint16(ifd[i+8 : i+10])}
		case cCompression:
			if datatype != dtShort || count != 1 {
				return 0, FormatError(fmt.Sprintf("Compression type: %v or count: %d not recognised", datatype, count))
			}
			imgDesc.Compression = d.bo.Uint16(ifd[i+8 : i+10])
		case cPhotometricInterpr:
			if datatype != dtShort || count != 1 {
				return 0, FormatError(fmt.Sprintf("PhotometricInterpretation type: %v or count: %d not recognised", datatype, count))
			}
			imgDesc.PhotometricInterpr = d.bo.Uint16(ifd[i+8 : i+10])
		case cSamplesPerPixel:
			if datatype != dtShort || count != 1 {
				return 0, FormatError(fmt.Sprintf("SamplesPerPixel type: %v or count: %d not recognised", datatype, count))
			}
			imgDesc.SamplesPerPixel = d.bo.Uint16(ifd[i+8 : i+10])
		case cSampleFormat:
			if datatype != dtShort {
				return 0, FormatError(fmt.Sprintf("SampleFormat type: %v not recognised", datatype))
			}
			imgDesc.SampleFormat = []uint16{d.bo.Uint16(ifd[i+8 : i+10])}
		case cPredictor:
			if datatype != dtShort {
				return 0, FormatError(fmt.Sprintf("SampleFormat type: %v not recognised", datatype))
			}
			imgDesc.Predictor = d.bo.Uint16(ifd[i+8 : i+10])
			if imgDesc.Predictor != 1 && imgDesc.Predictor != 2 {
				return 0, FormatError(fmt.Sprintf("Predictor other then 1=None or 2=Horizontal not implemented: %v", imgDesc.Predictor))
			}
		case cTileWidth:
			if count != 1 {
				return 0, FormatError(fmt.Sprintf("TileWidth count: %d not recognised", count))
			}
			switch datatype {
			case dtShort:
				imgDesc.TileWidth = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.TileWidth = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError(fmt.Sprintf("TileWidth type: %v not recognised", datatype))
			}
		case cTileLength:
			if count != 1 {
				return 0, FormatError(fmt.Sprintf("TileLength count: %d not recognised", count))
			}
			switch datatype {
			case dtShort:
				imgDesc.TileHeight = uint32(d.bo.Uint16(ifd[i+8 : i+10]))
			case dtLong:
				imgDesc.TileHeight = d.bo.Uint32(ifd[i+8 : i+12])
			default:
				return 0, FormatError(fmt.Sprintf("TileLength type: %v not recognised", datatype))
			}
		case cTileOffsets, cTileByteCounts:
			if datatype != dtLong {
				return 0, FormatError(fmt.Sprintf("TileOffsets or TileByteCounts type: %v not recognised", datatype))
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
		case GeoDoubleParamsTag:
			if datatype != dtFloat64 {
				return 0, FormatError(fmt.Sprintf("DoubleParamsTag type: %v not recognised", datatype))
			}
			var raw []byte
			// The IFD contains a pointer to the real value.
			raw = make([]byte, int(count)*8)
			d.ra.ReadAt(raw, int64(d.bo.Uint32(ifd[i+8:i+12])))

			dParams = make([]float64, count)
			for i := uint32(0); i < count; i++ {
				dParams[i] = math.Float64frombits(d.bo.Uint64(raw[8*i : 8*(i+1)]))
			}
		case GeoAsciiParamsTag:
			if datatype != dtASCII {
				return 0, FormatError(fmt.Sprintf("GeogASCIIParamsTag type: %v not recognised", datatype))
			}
			var raw []byte
			// The IFD contains a pointer to the real value.
			raw = make([]byte, int(count)*4)
			d.ra.ReadAt(raw, int64(d.bo.Uint32(ifd[i+8:i+12])))
			aParams = string(raw)
		case tGeoKeyDirectory:
			if datatype != dtShort || count < 4 {
				return 0, FormatError(fmt.Sprintf("GeoKeyDirectory type: %v or count: %d not recognised", datatype, count))
			}
			var raw []byte
			// The IFD contains a pointer to the real value.
			raw = make([]byte, int(count)*2)
			d.ra.ReadAt(raw, int64(d.bo.Uint32(ifd[i+8:i+12])))

			data := make([]uint16, count)
			for i := uint32(0); i < count; i++ {
				data[i] = d.bo.Uint16(raw[2*i : 2*(i+1)])
			}

			keyDirVersion := data[0]
			if keyDirVersion != 1 {
				return 0, FormatError(fmt.Sprintf("GeoKeyDirectory version: %d  not recognised", keyDirVersion))
			}
			numKeys := int(data[3])

			kEntries = make([]KeyEntry, numKeys)
			for i := 0; i<numKeys; i++ {
				kEntries[i].KeyID = data[4*(i+1)]
				kEntries[i].TIFFTagLocation = data[4*(i+1)+1]
				kEntries[i].Count = data[4*(i+1)+2]
				kEntries[i].ValueOffset = data[4*(i+1)+3]
			}
		case tModelPixelScale:
			if datatype != dtFloat64 || count != 3 {
				return 0, FormatError(fmt.Sprintf("ModelTiePoint type: %v or count: %d not recognised", datatype, count))
			}
			var raw []byte
			// The IFD contains a pointer to the real value.
			raw = make([]byte, int(count)*8)
			d.ra.ReadAt(raw, int64(d.bo.Uint32(ifd[i+8:i+12])))

			pixelScale = make([]float64, count)
			for i := uint32(0); i < count; i++ {
				pixelScale[i] = math.Float64frombits(d.bo.Uint64(raw[8*i : 8*(i+1)]))
			}
		case tModelTiepoint:
			if datatype != dtFloat64 {
				return 0, FormatError(fmt.Sprintf("ModelTiePoint type: %v not recognised", datatype))
			}
			var raw []byte
			// The IFD contains a pointer to the real value.
			raw = make([]byte, int(count)*8)
			d.ra.ReadAt(raw, int64(d.bo.Uint32(ifd[i+8:i+12])))

			tiePoint = make([]float64, count)
			for i := uint32(0); i < count; i++ {
				tiePoint[i] = math.Float64frombits(d.bo.Uint64(raw[8*i : 8*(i+1)]))
			}
		case tModelTransformation:
			return 0, fmt.Errorf("Time to implement ModelTransformation, this file uses it!")
		default:
			nonCaptTags = append(nonCaptTags, tag)
		}
	}
	log.Println("ImgDesc:", imgDesc)
	log.Println("Non captured tag:", nonCaptTags)
	geo, err := parseGeoKeyDirectory(kEntries, dParams, aParams)
	log.Println("tiePoint:", tiePoint)
	log.Println("pixelScale:", pixelScale)
	if err != nil {
		return 0, err
	}
	log.Println(geo)


	d.dsc = append(d.dsc, imgDesc)

	nextIFDOffset := ifdOffset + int64(2) + int64(numItems*12)
	if _, err := d.ra.ReadAt(p[0:4], nextIFDOffset); err != nil {
		return 0, FormatError("error reading IFD")
	}
	ifdOffset = int64(d.bo.Uint32(p[:4]))

	return ifdOffset, nil
}

func (d *decoder) readIFD() error {
	var err error
	p := make([]byte, 4)
	if _, err = d.ra.ReadAt(p, 4); err != nil {
		return err
	}
	ifdOffset := int64(d.bo.Uint32(p[0:4]))

	for ifdOffset != 0 {
		ifdOffset, err = d.parseIFD(ifdOffset)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *decoder) colorModel(level int) color.Model {
	cfg := d.dsc[level]

	// TODO get range in color modes dynamically from tiff file metadata?
	switch cfg.PhotometricInterpr {
	case pBlackIsZero:
		switch sampleFormat(cfg.SampleFormat[0]) {
		case uintSample:
			switch cfg.BitsPerSample[0] {
			case 8:
				return scicolor.GrayU8Model{0, 255}
			case 16:
				return scicolor.GrayU16Model{0, 65535}
			}
		case sintSample:
			switch cfg.BitsPerSample[0] {
			case 8:
				return scicolor.GrayS8Model{-128, 127}
			case 16:
				return scicolor.GrayS16Model{-32768, 32767}
			}
		}
	}

	return nil
}

// decode decodes the raw data of an image.
// It reads from d.buf and writes the strip or tile into dst.
func (d *decoder) decode(dst image.Image, level, xmin, ymin, xmax, ymax int) error {
	cfg := d.dsc[level]

	//Horizontal differencing
	if cfg.Predictor == 2 {
		off := 0
		switch cfg.BitsPerSample[0] {
		case 8:
			for y := 0; y < int(cfg.TileHeight); y++ {
				var v0 uint8
				for x := 0; x < int(cfg.TileWidth); x++ {
					v := d.buf[off]
					if x > 0 {
						v += v0
					}
					d.buf[off] = v
					v0 = v
					off++
				}
			}
		case 16:
			for y := 0; y < int(cfg.TileHeight); y++ {
				var v0 uint16
				for x := 0; x < int(cfg.TileWidth); x++ {
					v := d.bo.Uint16(d.buf[off:off+2])
					if x > 0 {
						v += v0
					}
					d.bo.PutUint16(d.buf[off:off+2], v)
					v0 = v
					off += 2
				}
			}
		default:
			return FormatError("Predictor not implemented for other bitsizes than 8 or 16")
		}
	}


	rMaxX := minInt(xmax, dst.Bounds().Max.X)
	rMaxY := minInt(ymax, dst.Bounds().Max.Y)

	if cfg.SamplesPerPixel != 1 {
		return FormatError("image data type not implemented")
	}

	off := 0
	switch img := dst.(type) {
	case *scimage.GrayU8:
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
	case *scimage.GrayU16:
		for y := ymin; y < rMaxY; y++ {
			for x := xmin; x < rMaxX; x++ {
				if off+2 > len(d.buf) {
					return errNoPixels
				}
				v := d.bo.Uint16(d.buf[off:off+2])
				off += 2
				img.SetGrayU16(x, y, scicolor.GrayU16{v, img.Min, img.Max})
			}
			if rMaxX == img.Bounds().Max.X {
				off += 2 * (xmax - img.Bounds().Max.X)
			}
		}
	case *scimage.GrayS8:
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
	case *scimage.GrayS16:
		for y := ymin; y < rMaxY; y++ {
			for x := xmin; x < rMaxX; x++ {
				if off+2 > len(d.buf) {
					return errNoPixels
				}
				v := int16(d.bo.Uint16(d.buf[off:off+2]))
				off += 2
				img.SetGrayS16(x, y, scicolor.GrayS16{v, img.Min, img.Max})
			}
			if rMaxX == img.Bounds().Max.X {
				off += 2 * (xmax - img.Bounds().Max.X)
			}
		}
	default:
		return FormatError("malformed header")
	}

	return nil
}

func decodeLevelSubImage(d decoder, level int, rect image.Rectangle) (img image.Image, err error) {
	cfg := d.dsc[level]

	blockPadding := false
	blocksAcross := 1
	blocksDown := 1

	if cfg.ImageWidth == 0 || cfg.ImageHeight == 0 {
		return nil, FormatError("unexpected image dimensions")
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

	switch cfg.BitsPerSample[0] {
	case 0:
		return nil, FormatError("BitsPerSample must not be 0")
	case 8, 16:
		// Nothing to do, these are accepted by this implementation.
	default:
		return nil, UnsupportedError(fmt.Sprintf("BitsPerSample of %v", cfg.BitsPerSample))
	}

	imgRect := image.Rect(0, 0, int(cfg.ImageWidth), int(cfg.ImageHeight)).Intersect(rect)
	if imgRect.Empty() {
		return nil, fmt.Errorf("The rectangle provided does not intersect the image")
	}

	switch v := d.colorModel(level).(type) {
	case scicolor.GrayU8Model:
		img = scimage.NewGrayU8(imgRect, v.Min, v.Max)
	case scicolor.GrayU16Model:
		img = scimage.NewGrayU16(imgRect, v.Min, v.Max)
	case scicolor.GrayS8Model:
		img = scimage.NewGrayS8(imgRect, v.Min, v.Max)
	case scicolor.GrayS16Model:
		img = scimage.NewGrayS16(imgRect, v.Min, v.Max)
	default:
		return nil, FormatError("image data type not implemented")
	}

	for i := imgRect.Bounds().Min.X/int(cfg.TileWidth); i <= imgRect.Bounds().Max.X/int(cfg.TileWidth); i++ {
		blkW := int(cfg.TileWidth)
		if !blockPadding && i == blocksAcross-1 && cfg.ImageWidth%cfg.TileWidth != 0 {
			blkW = int(cfg.ImageWidth % cfg.TileWidth)
		}
		for j := imgRect.Bounds().Min.Y/int(cfg.TileWidth); j <= imgRect.Bounds().Max.Y/int(cfg.TileWidth); j++ {
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
			case cLZW:
				r := lzw.NewReader(io.NewSectionReader(d.ra, offset, n), lzw.MSB, 8)
				d.buf, err = ioutil.ReadAll(r)
				r.Close()
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
func DecodeLevelSubImage(r io.Reader, level int, rect image.Rectangle) (img image.Image, err error) {
	d, err := newDecoder(r)
	if err != nil {
		return nil, err
	}
	err = d.readIFD()
	if err != nil {
		return nil, err
	}

	return decodeLevelSubImage(d, level, rect)
}

func DecodeLevel(r io.Reader, level int) (img image.Image, err error) {
	d, err := newDecoder(r)
	if err != nil {
		return nil, err
	}
	err = d.readIFD()
	if err != nil {
		return nil, err
	}

	cfg := d.dsc[level]
	rect := image.Rect(0, 0, int(cfg.ImageWidth), int(cfg.ImageHeight))

	return decodeLevelSubImage(d, level, rect)
}

func Decode(r io.Reader) (img image.Image, err error) {
	d, err := newDecoder(r)
	if err != nil {
		return nil, err
	}
	err = d.readIFD()
	if err != nil {
		return nil, err
	}

	cfg := d.dsc[0]
	rect := image.Rect(0, 0, int(cfg.ImageWidth), int(cfg.ImageHeight))

	return decodeLevelSubImage(d, 0, rect)
}

func DecodeConfigLevel(r io.Reader, level int) (image.Config, error) {
	d, err := newDecoder(r)
	if err != nil {
		return image.Config{}, err
	}
	err = d.readIFD()
	if err != nil {
		return image.Config{}, err
	}

	cfg := d.dsc[level]

	return image.Config{ColorModel: d.colorModel(level), Width: int(cfg.ImageWidth), Height: int(cfg.ImageHeight)}, nil
}

// DecodeConfig returns the color model and dimensions of a TIFF image without
// decoding the entire image.
func DecodeConfig(r io.Reader) (image.Config, error) {
	return DecodeConfigLevel(r, 0)
}

func init() {
	image.RegisterFormat("cog", leHeader, Decode, DecodeConfig)
	image.RegisterFormat("cog", beHeader, Decode, DecodeConfig)
}
