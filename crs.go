package gocog

import "fmt"

type ModelType string
type RasterType string
type GeographicType string
type ProjCoordTrans string
type GeogGeodeticDatum string
type GeogEllipsoid string
type GeogAngularUnits string
type ProjCSTType string
type ProjLinearUnits string
type Projection string

const (
	Projected ModelType = "Projected"
	Geographic ModelType = "Geographic"
	Geocentric ModelType = "Geocentric"

	PixelIsArea RasterType = "PixelIsArea"
	PixelIsPoint RasterType = "PixelIsPoint"

	//Section 6.3.1.3 codes
	LinearMeter ProjLinearUnits = "Meter"

	//Section 6.3.1.4 codes
	AngularRadian GeogAngularUnits = "Radian"
	AngularDegree GeogAngularUnits = "Degree"

	//Section 6.3.2.1 codes
	GCS_WGS84 GeographicType = "WGS_84"
	UserDefinedGeogType GeographicType = "user-defined"

	//Section 6.3.2.2 codes
	DatumWGS84 GeogGeodeticDatum = "WGS_84"
	UserDefinedGeodDatum GeogGeodeticDatum = "user-defined"

	//Section 6.3.2.3 codes
	EllipseWGS84 GeogEllipsoid = "WGS_84"
	EllipseSphere GeogEllipsoid = "Sphere"
	UserDefinedGeogEllipsoid GeogEllipsoid = "user-defined"

	//Section 6.3.3.2 codes
	UserDefinedProjection Projection = "user-defined"


	//Section 6.3.3.3 codes
	EPSG3857 ProjCSTType = "EPSG:3857"
	PCS_WGS84_UTM_zone_1N ProjCSTType = "WGS84_UTM_zone_1N"
	PCS_WGS84_UTM_zone_33N ProjCSTType = "WGS84_UTM_zone_33N"
	UserDefinedCSTType ProjCSTType = "user-defined"

	//Section 6.3.3.3 codes
	CTTransverseMercator ProjCoordTrans = "TransverseMercator"
	CTAlbersEqualArea ProjCoordTrans = "AlbersEqualArea"
	CTSinusoidal ProjCoordTrans = "Sinusoidal"

)

type GeoCode struct {
	ModelType
	RasterType
	Citation string

	GeographicType
	GeogCitation string
	GeogGeodeticDatum
	GeogAngularUnits
	GeogEllipsoid
	GeogSemiMajorAxis float64
	GeogSemiMinorAxis float64
	GeogPrimeMeridianLong float64

	ProjCSTType
	Projection
	ProjCoordTrans
	ProjLinearUnits
	ProjFalseEasting float64
	ProjFalseNorthing float64
	ProjCenterLong float64
}

type KeyEntry struct { 
	KeyID, TIFFTagLocation, Count, ValueOffset uint16
	}

func (g *GeoCode) extract(k KeyEntry, dParams []float64, aParams string) error {
	switch k.KeyID {
	case GTModelTypeGeoKey:
		switch k.ValueOffset {
		case 1:
			g.ModelType = Projected
		case 2:
			g.ModelType = Geographic
		case 3:
			g.ModelType = Geocentric
		default:
			return FormatError(fmt.Sprintf("ModelType: %d not recognised", k.ValueOffset))
		}
	case GTRasterTypeGeoKey:
		switch k.ValueOffset {
		case 1:
			g.RasterType = PixelIsArea
		case 2:
			g.RasterType = PixelIsPoint
		default:
			return FormatError(fmt.Sprintf("RasterType: %d not recognised", k.ValueOffset))
		}
	case GTCitationGeoKey:
		if k.TIFFTagLocation == GeoAsciiParamsTag {
			g.Citation = aParams[k.ValueOffset:k.ValueOffset+k.Count]

		}
	case GeographicTypeGeoKey:
		switch k.ValueOffset {
		case 4326:
			g.GeographicType = GCS_WGS84
		case 32767:
			g.GeographicType = UserDefinedGeogType
		default:
			return FormatError(fmt.Sprintf("GeographicType: %d not recognised", k.ValueOffset))
		}
	case GeogCitationGeoKey:
		if k.TIFFTagLocation == GeoAsciiParamsTag {
			g.GeogCitation = aParams[k.ValueOffset:k.ValueOffset+k.Count]
		}
	case GeogGeodeticDatumGeoKey:
		switch k.ValueOffset {
		case 6326:
			g.GeogGeodeticDatum = DatumWGS84
		case 32767:
			g.GeogGeodeticDatum = UserDefinedGeodDatum
		default:
			return FormatError(fmt.Sprintf("GeogGeodeticDatum: %d not recognised", k.ValueOffset))
		}
	case GeogAngularUnitsGeoKey:
		switch k.ValueOffset {
		case 9101:
			g.GeogAngularUnits = AngularRadian
		case 9102:
			g.GeogAngularUnits = AngularDegree
		default:
			return FormatError(fmt.Sprintf("GeogAngularUnits: %d not recognised", k.ValueOffset))
		}
	case GeogEllipsoidGeoKey:
		switch k.ValueOffset {
		case 7030:
			g.GeogEllipsoid = EllipseWGS84
		case 7035:
			g.GeogEllipsoid = EllipseSphere
		case 32767:
			g.GeogEllipsoid = UserDefinedGeogEllipsoid
		default:
			return FormatError(fmt.Sprintf("GeogEllipsoid: %d not recognised", k.ValueOffset))
		}
	case GeogSemiMajorAxisGeoKey:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.GeogSemiMajorAxis = dParams[k.ValueOffset]
		}
	case GeogSemiMinorAxisGeoKey:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.GeogSemiMinorAxis = dParams[k.ValueOffset]
		}
	case GeogPrimeMeridianLongGeoKey:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.GeogPrimeMeridianLong = dParams[k.ValueOffset]
		}

	case ProjectedCSTypeGeoKey:
		switch k.ValueOffset {
		case 3857:
			g.ProjCSTType = EPSG3857
		case 32601:
			g.ProjCSTType = PCS_WGS84_UTM_zone_1N
		case 32633:
			g.ProjCSTType = PCS_WGS84_UTM_zone_33N
		case 32767:
			g.ProjCSTType = UserDefinedCSTType
		default:
			return FormatError(fmt.Sprintf("ProjectedCSType: %d not recognised", k.ValueOffset))
		}
	case ProjectionGeoKey:
		switch k.ValueOffset {
		case 32767:
			g.Projection = UserDefinedProjection
		default:
			return FormatError(fmt.Sprintf("ProjectionGeoKey: %d not recognised", k.ValueOffset))
		}
	case ProjCoordTransGeoKey:
		switch k.ValueOffset {
		case 1:
			g.ProjCoordTrans = CTTransverseMercator
		case 11:
			g.ProjCoordTrans = CTAlbersEqualArea
		case 24:
			g.ProjCoordTrans = CTSinusoidal
		default:
			return FormatError(fmt.Sprintf("ProjCoordTrans: %d not recognised", k.ValueOffset))
		}

	case ProjLinearUnitsGeoKey:
		switch k.ValueOffset {
		case 9001:
			g.ProjLinearUnits = LinearMeter
		default:
			return FormatError(fmt.Sprintf("ProjLinearUnits: %d not recognised", k.ValueOffset))
		}
	case ProjFalseEastingGeoKey:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.ProjFalseEasting = dParams[k.ValueOffset]
		}
	case ProjFalseNorthingGeoKey:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.ProjFalseNorthing = dParams[k.ValueOffset]
		}
	case ProjCenterLongGeoKey:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.ProjCenterLong = dParams[k.ValueOffset]
		}
	default:
		return FormatError(fmt.Sprintf("GeoKey: %d not implemented", k.ValueOffset))
	}

	return nil
}

func parseGeoKeyDirectory(kEntries []KeyEntry, dParams []float64, aParams string) (GeoCode, error) {
	gc := GeoCode{}
	for _, kEntry := range kEntries {
		err := gc.extract(kEntry, dParams, aParams)
		if err != nil {
			return gc, err
		}
	}
	return gc, nil
}
