package gocog

import "fmt"

type GeoCode struct {
	ModelType uint16
	RasterType uint16
	Citation string
	GeographicType uint16
	GeogCitation string
	GeogGeodeticDatum uint16
	GeogAngularUnits uint16
	GeogEllipsoid uint16
	GeogSemiMajorAxis float64
	GeogSemiMinorAxis float64
	GeogPrimeMeridianLong float64
	ProjCSTType uint16
	Proj uint16
	ProjCoordTrans uint16
	ProjLinearUnits uint16
	ProjFalseEasting float64
	ProjFalseNorthing float64
	ProjCenterLong float64
}

type KeyEntry struct { 
	KeyID, TIFFTagLocation, Count, ValueOffset uint16
	}

func (g *GeoCode) extract(k KeyEntry, dParams []float64, aParams string) error {
	switch k.KeyID {
	case 1024:
		g.ModelType = k.ValueOffset
	case 1025:
		g.RasterType = k.ValueOffset
	case 1026:
		if k.TIFFTagLocation == GeoAsciiParamsTag {
			g.Citation = aParams[k.ValueOffset:k.ValueOffset+k.Count]

		}
	case 2048:
		g.GeographicType = k.ValueOffset
	case 2049:
		if k.TIFFTagLocation == GeoAsciiParamsTag {
			g.GeogCitation = aParams[k.ValueOffset:k.ValueOffset+k.Count]
		}
	case 2050:
		g.GeogGeodeticDatum = k.ValueOffset
	case 2054:
		g.GeogAngularUnits = k.ValueOffset
	case 2056:
		g.GeogEllipsoid = k.ValueOffset
	case 2057:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.GeogSemiMajorAxis = dParams[k.ValueOffset]
		}
	case 2058:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.GeogSemiMinorAxis = dParams[k.ValueOffset]
		}
	case 2061:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.GeogPrimeMeridianLong = dParams[k.ValueOffset]
		}
	case 3072:
		g.ProjCSTType = k.ValueOffset
	case 3074:
		g.Proj = k.ValueOffset
	case 3075:
		g.ProjCoordTrans = k.ValueOffset
	case 3076:
		g.ProjLinearUnits = k.ValueOffset
	case 3082:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.ProjFalseEasting = dParams[k.ValueOffset]
		}
	case 3083:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.ProjFalseNorthing = dParams[k.ValueOffset]
		}
	case 3088:
		if k.TIFFTagLocation == GeoDoubleParamsTag {
			g.ProjCenterLong = dParams[k.ValueOffset]
		}
	default:
		fmt.Println("Not processed", k)
	}

	return nil
}

func parseGeoKeyDirectory(kEntries []KeyEntry, dParams []float64, aParams string) GeoCode {
	gc := GeoCode{}
	for _, kEntry := range kEntries {
		gc.extract(kEntry, dParams, aParams)
	}
	return gc
}
