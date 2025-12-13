package translate

// CollectionToDataset maps STAC collection IDs to ASF dataset names.
// Based on the design document section 3.2.
var CollectionToDataset = map[string][]string{
	"sentinel-1":        {"SENTINEL-1"},
	"sentinel-1-bursts": {"SLC-BURST"},
	"opera-s1":          {"OPERA-S1"},
	"alos-palsar":       {"ALOS PALSAR"},
	"alos-avnir-2":      {"ALOS AVNIR-2"},
	"radarsat-1":        {"RADARSAT-1"},
	"ers":               {"ERS"},
	"jers-1":            {"JERS-1"},
	"seasat":            {"SEASAT"},
	"uavsar":            {"UAVSAR"},
	"airsar":            {"AIRSAR"},
	"sir-c":             {"SIR-C"},
	"smap":              {"SMAP"},
	"aria-s1-gunw":      {"ARIA S1 GUNW"},
	"nisar":             {"NISAR"},
}

// DatasetToCollection maps ASF dataset names to STAC collection IDs (reverse mapping).
var DatasetToCollection = map[string]string{
	"SENTINEL-1":    "sentinel-1",
	"SLC-BURST":     "sentinel-1-bursts",
	"OPERA-S1":      "opera-s1",
	"ALOS PALSAR":   "alos-palsar",
	"ALOS AVNIR-2":  "alos-avnir-2",
	"RADARSAT-1":    "radarsat-1",
	"ERS":           "ers",
	"JERS-1":        "jers-1",
	"SEASAT":        "seasat",
	"UAVSAR":        "uavsar",
	"AIRSAR":        "airsar",
	"SIR-C":         "sir-c",
	"SMAP":          "smap",
	"ARIA S1 GUNW":  "aria-s1-gunw",
	"NISAR":         "nisar",
}

// GetASFDatasets returns the ASF dataset names for a given STAC collection ID.
// Returns the datasets and a boolean indicating if the collection was found.
func GetASFDatasets(collectionID string) ([]string, bool) {
	datasets, ok := CollectionToDataset[collectionID]
	return datasets, ok
}

// GetCollectionID returns the STAC collection ID for a given ASF dataset name.
// Returns the collection ID and a boolean indicating if the dataset was found.
func GetCollectionID(asfDataset string) (string, bool) {
	collectionID, ok := DatasetToCollection[asfDataset]
	return collectionID, ok
}
