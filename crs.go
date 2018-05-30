package gocog

import (
	"fmt"
	"strings"
	"regexp"
)

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
	LinearMeter ProjLinearUnits = "metre"

	//Section 6.3.1.4 codes
	AngularRadian GeogAngularUnits = "radian"
	AngularDegree GeogAngularUnits = "degree"

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

type GeoData struct {
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
	GeogPrimeMeridian string
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

func (g *GeoData) extract(k KeyEntry, dParams []float64, aParams string) error {
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
		if k.TIFFTagLocation != GeoAsciiParamsTag {
			return FormatError(fmt.Sprintf("GTCitationGeoKey is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.Citation = aParams[k.ValueOffset:k.ValueOffset+k.Count]
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
		if k.TIFFTagLocation != GeoAsciiParamsTag {
			return FormatError(fmt.Sprintf("GeogCitationGeoKey is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.GeogCitation = aParams[k.ValueOffset:k.ValueOffset+k.Count]
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
		if k.TIFFTagLocation != GeoDoubleParamsTag {
			return FormatError(fmt.Sprintf("GeogSemiMajorAxis is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.GeogSemiMajorAxis = dParams[k.ValueOffset]
	case GeogSemiMinorAxisGeoKey:
		if k.TIFFTagLocation != GeoDoubleParamsTag {
			return FormatError(fmt.Sprintf("GeogSemiMinorAxis is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.GeogSemiMinorAxis = dParams[k.ValueOffset]
	case GeogPrimeMeridianGeoKey:
		if k.TIFFTagLocation != GeoAsciiParamsTag {
			return FormatError(fmt.Sprintf("GeogPrimeMeridianGeoKey is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.GeogPrimeMeridian = aParams[k.ValueOffset : k.ValueOffset+k.Count]
	case GeogPrimeMeridianLongGeoKey:
		if k.TIFFTagLocation != GeoDoubleParamsTag {
			return FormatError(fmt.Sprintf("GeogPrimeMeridianLongGeoKey is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.GeogPrimeMeridianLong = dParams[k.ValueOffset]
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
		if k.TIFFTagLocation != GeoDoubleParamsTag {
			return FormatError(fmt.Sprintf("ProjFalseEastingGeoKey is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.ProjFalseEasting = dParams[k.ValueOffset]
	case ProjFalseNorthingGeoKey:
		if k.TIFFTagLocation != GeoDoubleParamsTag {
			return FormatError(fmt.Sprintf("ProjFalseNorthingGeoKey is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.ProjFalseNorthing = dParams[k.ValueOffset]
	case ProjCenterLongGeoKey:
		if k.TIFFTagLocation != GeoDoubleParamsTag {
			return FormatError(fmt.Sprintf("ProjCenterLongGeoKey is pointing to an unexpected location: %d ", k.TIFFTagLocation))
		}
		g.ProjCenterLong = dParams[k.ValueOffset]
	default:
		return FormatError(fmt.Sprintf("GeoKey: %d not implemented", k.ValueOffset))
	}

	return nil
}

func parseGeoKeyDirectory(kEntries []KeyEntry, dParams []float64, aParams string) (GeoData, error) {
	gc := GeoData{}
	for _, kEntry := range kEntries {
		err := gc.extract(kEntry, dParams, aParams)
		if err != nil {
			return gc, err
		}
	}


	return gc, nil
}

type Citation struct {
	GCS string
	Datum string
	Ellipsoid string
	Primem string
}

func parseGeoAsciiParams(s string) Citation {
	fmt.Println(s)
	rawParams := strings.Split(s, "|")
	gcs, _ := regexp.Compile(`\s*GCS\sName\s*=\s*(?P<name>[a-zA-Z-_ +()0-9]+)\s*`)
	datum, _ := regexp.Compile(`\s*Datum\s*=\s*(?P<name>[a-zA-Z-_ +()0-9]+)\s*`)
	ellps, _ := regexp.Compile(`\s*Ellipsoid\s*=\s*(?P<name>[a-zA-Z-_ +()0-9]+)\s*`)
	primem, _ := regexp.Compile(`\s*Primem\s*=\s*(?P<name>[a-zA-Z-_ +()0-9]+)\s*`)

	cit := Citation{}
	for _, rawParam := range rawParams {
		if res := gcs.FindStringSubmatch(rawParam); len(res) == 2 {
			cit.GCS = res[1]
		}
		if res := datum.FindStringSubmatch(rawParam); len(res) == 2 {
			cit.Datum = res[1]
		}
		if res := ellps.FindStringSubmatch(rawParam); len(res) == 2 {
			cit.Ellipsoid = res[1]
		}
		if res := primem.FindStringSubmatch(rawParam); len(res) == 2 {
			cit.Primem = res[1]
		}
	}

	return cit
}

func (gd GeoData) WKT() (string, error) {
	cit := parseGeoAsciiParams(gd.GeogCitation)

	str := ""
	if gd.ModelType != Projected {
		return str, fmt.Errorf("Only Projected CRS are implemented")
	}

	str += `PROJCS["unnamed",`

	str += fmt.Sprintf(`GEOGCS["%s",`, cit.GCS)

	str += "DATUM["
	if cit.Datum  != "" {
		str += fmt.Sprintf(`"%s",`, cit.Datum)
	} else {
		str += fmt.Sprintf(`"%s",`, string(gd.GeogGeodeticDatum))
	}

	str += "SPHEROID["
	if cit.Ellipsoid != "" {
		str += fmt.Sprintf(`"%s",`, cit.Ellipsoid)
	} else {
		str += fmt.Sprintf(`"%s",`, string(gd.GeogEllipsoid))
	}
	str += fmt.Sprintf("%f,", gd.GeogSemiMajorAxis)
	str += fmt.Sprintf("%f]],", gd.GeogSemiMajorAxis-gd.GeogSemiMinorAxis)

	str += "PRIMEM["
	if cit.Primem != "" {
		str += fmt.Sprintf("%s,", cit.Primem)
	} else {
		str += fmt.Sprintf("%s,", string(gd.GeogPrimeMeridian))
	}
	str += fmt.Sprintf("%f],", gd.GeogPrimeMeridianLong)

	str += fmt.Sprintf(`UNIT["%s",%f]],`, string(gd.GeogAngularUnits), 0.0174532925199433)

	str += fmt.Sprintf(`PROJECTION["%s"],`, gd.ProjCoordTrans)
	str += fmt.Sprintf(`PARAMETER["%s",%f],`, "longitude_of_center", gd.ProjCenterLong)
	str += fmt.Sprintf(`PARAMETER["%s",%f],`, "false_easting", gd.ProjFalseEasting)
	str += fmt.Sprintf(`PARAMETER["%s",%f],`, "false_northing", gd.ProjFalseNorthing)

	str += fmt.Sprintf(`UNIT["%s",%f]]`, string(gd.ProjLinearUnits), 1.0)

	return str, nil
}

