// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocog

// A tiff image file contains one or more images. The metadata
// of each image is contained in an Image File Directory (IFD),
// which contains entries of 12 bytes each and is described
// on page 14-16 of the specification. An IFD entry consists of
//
//  - a tag, which describes the signification of the entry,
//  - the data type and length of the entry,
//  - the data itself or a pointer to it if it is more than 4 bytes.
//
// The presence of a length means that each IFD is effectively an array.

const (
	leHeader = "II\x2A\x00" // Header for little-endian files.
	beHeader = "MM\x00\x2A" // Header for big-endian files.

	ifdLen = 12 // Length of an IFD entry in bytes.
)

// Data types (p. 14-16 of the spec).
const (
	dtByte      = 1
	dtASCII     = 2
	dtShort     = 3
	dtLong      = 4
	dtRational  = 5
	dtInt8      = 6
	dtUndefined = 7
	dtInt16     = 8
	dtInt32     = 9
	dtSRational = 10
	dtFloat32   = 11
	dtFloat64   = 12
)

// The length of one instance of each data type in bytes.
var lengths = [...]uint32{0, 1, 1, 2, 4, 8, 1, 0, 2, 4, 8, 4, 8}

const (
	cNewSubfileType      = 254
	cImageWidth          = 256
	cImageLength         = 257
	cBitsPerSample       = 258
	cCompression         = 259
	cPhotometricInterpr  = 262
	cSamplesPerPixel     = 277
	cPlanarConfiguration = 284

	cPredictor    = 317

	cTileWidth           = 322
	cTileLength          = 323
	cTileOffsets         = 324
	cTileByteCounts      = 325
	cSampleFormat        = 339
)



const (
	// GDAL tags
	tGDALMetadata = 42112
	tGDALNoData   = 42113

	// GeoTIFF tags
	tModelPixelScale     = 33550
	tModelTiepoint       = 33922
	tModelTransformation = 34264
	tGeoKeyDirectory     = 34735

	//This tag is used to store all of the DOUBLE valued GeoKeys, referenced by the
	//GeoKeyDirectoryTag. The meaning of any value of this double array is determined
	//from the GeoKeyDirectoryTag reference pointing to it. FLOAT values should first
	//be converted to DOUBLE and stored here.
	GeoDoubleParamsTag = 34736 // Type = DOUBLE (IEEE Double precision) N = variable

	//This tag is used to store all of the ASCII valued GeoKeys, referenced by the
	//GeoKeyDirectoryTag. Since keys use offsets into tags, any special comments
	//may be placed at the beginning of this tag. For the most part, the only keys
	//that are ASCII valued are "Citation" keys, giving documentation and references
	//for obscure projections, datums, etc.
	GeoAsciiParamsTag = 34737 // Type = ASCII N = variable
)


/*
// Tags (see p. 28-41 of the spec).
const (
	tImageWidth                = 256
	tImageLength               = 257
	tBitsPerSample             = 258
	tCompression               = 259
	tPhotometricInterpretation = 262

	tStripOffsets    = 273
	tSamplesPerPixel = 277
	tRowsPerStrip    = 278
	tStripByteCounts = 279

	tTileWidth      = 322
	tTileLength     = 323
	tTileOffsets    = 324
	tTileByteCounts = 325

	tOrientation    = 274
	tXResolution    = 282
	tYResolution    = 283
	tXPosition      = 286
	tYPosition      = 287
	tResolutionUnit = 296

	tPredictor    = 317
	tColorMap     = 320
	tExtraSamples = 338
	tSampleFormat = 339

)
*/

// Key ID Summary
const (
	//GeoTIFF Configuration Keys
	GTModelTypeGeoKey  = 1024 // Section 6.3.1.1 Codes
	GTRasterTypeGeoKey = 1025 // Section 6.3.1.2 Codes
	GTCitationGeoKey   = 1026 // documentation

	// Geographic CS Parameter Keys
	GeographicTypeGeoKey        = 2048 // Section 6.3.2.1 Codes
	GeogCitationGeoKey          = 2049 // documentation
	GeogGeodeticDatumGeoKey     = 2050 // Section 6.3.2.2 Codes
	GeogPrimeMeridianGeoKey     = 2051 // Section 6.3.2.4 codes
	GeogLinearUnitsGeoKey       = 2052 // Section 6.3.1.3 Codes
	GeogLinearUnitSizeGeoKey    = 2053 // meters
	GeogAngularUnitsGeoKey      = 2054 // Section 6.3.1.4 Codes
	GeogAngularUnitSizeGeoKey   = 2055 // radians
	GeogEllipsoidGeoKey         = 2056 // Section 6.3.2.3 Codes
	GeogSemiMajorAxisGeoKey     = 2057 // GeogLinearUnits
	GeogSemiMinorAxisGeoKey     = 2058 // GeogLinearUnits
	GeogInvFlatteningGeoKey     = 2059 // ratio
	GeogAzimuthUnitsGeoKey      = 2060 // Section 6.3.1.4 Codes
	GeogPrimeMeridianLongGeoKey = 2061 // GeogAngularUnit

	// Projected CS Parameter Keys
	ProjectedCSTypeGeoKey          = 3072 // Section 6.3.3.1 codes
	PCSCitationGeoKey              = 3073 // documentation
	ProjectionGeoKey               = 3074 // Section 6.3.3.2 codes
	ProjCoordTransGeoKey           = 3075 // Section 6.3.3.3 codes
	ProjLinearUnitsGeoKey          = 3076 // Section 6.3.1.3 codes
	ProjLinearUnitSizeGeoKey       = 3077 // meters
	ProjStdParallel1GeoKey         = 3078 // GeogAngularUnit
	ProjStdParallel2GeoKey         = 3079 // GeogAngularUnit
	ProjNatOriginLongGeoKey        = 3080 // GeogAngularUnit
	ProjNatOriginLatGeoKey         = 3081 // GeogAngularUnit
	ProjFalseEastingGeoKey         = 3082 // ProjLinearUnits
	ProjFalseNorthingGeoKey        = 3083 // ProjLinearUnits
	ProjFalseOriginLongGeoKey      = 3084 // GeogAngularUnit
	ProjFalseOriginLatGeoKey       = 3085 // GeogAngularUnit
	ProjFalseOriginEastingGeoKey   = 3086 // ProjLinearUnits
	ProjFalseOriginNorthingGeoKey  = 3087 // ProjLinearUnits
	ProjCenterLongGeoKey           = 3088 // GeogAngularUnit
	ProjCenterLatGeoKey            = 3089 // GeogAngularUnit
	ProjCenterEastingGeoKey        = 3090 // ProjLinearUnits
	ProjCenterNorthingGeoKey       = 3091 // ProjLinearUnits
	ProjScaleAtNatOriginGeoKey     = 3092 // ratio
	ProjScaleAtCenterGeoKey        = 3093 // ratio
	ProjAzimuthAngleGeoKey         = 3094 // GeogAzimuthUnit
	ProjStraightVertPoleLongGeoKey = 3095 // GeogAngularUnit

)

/*
const (
	CT_TransverseMercator             = 1
	CT_TransvMercator_Modified_Alaska = 2
	CT_ObliqueMercator                = 3
	CT_ObliqueMercator_Laborde        = 4
	CT_ObliqueMercator_Rosenmund      = 5
	CT_ObliqueMercator_Spherical      = 6
	CT_Mercator                       = 7
	CT_LambertConfConic_2SP           = 8
	CT_LambertConfConic_Helmert       = 9
	CT_LambertAzimEqualArea           = 10
	CT_AlbersEqualArea                = 11
	CT_AzimuthalEquidistant           = 12
	CT_EquidistantConic               = 13
	CT_Stereographic                  = 14
	CT_PolarStereographic             = 15
	CT_ObliqueStereographic           = 16
	CT_Equirectangular                = 17
	CT_CassiniSoldner                 = 18
	CT_Gnomonic                       = 19
	CT_MillerCylindrical              = 20
	CT_Orthographic                   = 21
	CT_Polyconic                      = 22
	CT_Robinson                       = 23
	CT_Sinusoidal                     = 24
	CT_VanDerGrinten                  = 25
	CT_NewZealandMapGrid              = 26
	CT_TransvMercator_SouthOriented   = 27
)
*/

// Compression types (defined in various places in the spec and supplements).
const (
	cNone       = 1
	cCCITT      = 2
	cG3         = 3 // Group 3 Fax.
	cG4         = 4 // Group 4 Fax.
	cLZW        = 5
	cJPEGOld    = 6 // Superseded by cJPEG.
	cJPEG       = 7
	cDeflate    = 8 // zlib compression.
	cPackBits   = 32773
	cDeflateOld = 32946 // Superseded by cDeflate.
)

// Photometric interpretation values (see p. 37 of the spec).
const (
	pWhiteIsZero = 0
	pBlackIsZero = 1
	pRGB         = 2
	pPaletted    = 3
	pTransMask   = 4 // transparency mask
	pCMYK        = 5
	pYCbCr       = 6
	pCIELab      = 8
)

// Values for the tPredictor tag (page 64-65 of the spec).
const (
	prNone       = 1
	prHorizontal = 2
)

// Values for the tResolutionUnit tag (page 18).
const (
	resNone    = 1
	resPerInch = 2 // Dots per inch.
	resPerCM   = 3 // Dots per centimeter.
)

// sampleFormat represents the mode of the image.
type sampleFormat int

const (
	_ sampleFormat = iota
	uintSample
	sintSample
	ieeefpSample
	voidSample
)

// imageMode represents the mode of the image.
type imageMode int

const (
	mBilevel imageMode = iota
	mPaletted
	mGray
	mGrayInvert
	mRGB
	mRGBA
	mNRGBA
)

// CompressionType describes the type of compression used in Options.
type CompressionType int

const (
	Uncompressed CompressionType = iota
	Deflate
)

// specValue returns the compression type constant from the TIFF spec that
// is equivalent to c.
func (c CompressionType) specValue() uint32 {
	switch c {
	case Deflate:
		return cDeflate
	}
	return cNone
}
