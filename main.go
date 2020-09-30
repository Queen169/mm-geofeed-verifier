package main

/*
This script is meant to help verify 'bulk correction' files for submission
to MaxMind. The files are expected to (mostly) follow the format provided by the RFC at
https://tools.ietf.org/html/draft-google-self-published-geofeeds-09
Region codes without the country prefix are accepted. eg, 'NY' is allowed, along with
'US-NY' for the state of New York in the United States.
Beyond verifying that the format of the data is correct, the script will also compare
the corrections against a given MMDB, reporting on how many corrections differ from
the contents in the database.
*/

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"path/filepath"
	"strings"

	"github.com/TomOnTime/utfutil"

	geoip2 "github.com/oschwald/geoip2-golang"
)

func main() {
	geofeedFilename, mmdbFilename, err := parseArgs()
	if err != nil {
		return err
	}

	db, err := geoip2.Open(mmdbFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Println(err)
		}
	}()

	totalCount, correctionCount := 0, 0
	geofeedFH, err := utfutil.OpenFile(geofeedFilename, utfutil.UTF8)
	if err != nil {
		log.Panic(err)
	}

	csvReader := csv.NewReader(geofeedFH)
	csvReader.ReuseRecord = true
	csvReader.Comment = '#'
	csvReader.FieldsPerRecord = 5
	csvReader.TrimLeadingSpace = true
	defer func() {
		if err := geofeedFH.Close(); err != nil {
			log.Println(err)
		}
	}()

	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			geofeedFH.Close() //nolint: gosec, errcheck
			log.Panic(err)
		}
		totalCount++
		currentCorrectionCount, err := verifyCorrection(row, db)
		if err != nil {
			geofeedFH.Close() //nolint: gosec, errcheck
			log.Panic(err)
		}
		correctionCount += currentCorrectionCount
	}
	if err != nil && err != io.EOF {
		log.Panicf("Failed to read file: %s", err)
	}
	fmt.Printf(
		"\nOut of %d potential corrections, %d may be different than our current mappings\n",
		totalCount,
		correctionCount,
	)
}

func verifyCorrection(correction []string, db *geoip2.Reader) (int, error) {
	/*
	   0: network (CIDR or single IP)
	   1: ISO-3166 country code
	   2: ISO-3166-2 region code
	   3: city name
	   4: postal code
	*/
	networkOrIP := correction[0]
	if !(strings.Contains(networkOrIP, "/")) {
		if strings.Contains(networkOrIP, ":") {
			networkOrIP += "/64"
		} else {
			networkOrIP += "/32"
		}
	}
	network, _, err := net.ParseCIDR(networkOrIP)
	if err != nil {
		return 0, err
	}
	mmdbRecord, err := db.City(network)
	if err != nil {
		return 0, err
	}
	firstSubdivision := ""
	if len(mmdbRecord.Subdivisions) > 0 {
		firstSubdivision = mmdbRecord.Subdivisions[0].IsoCode
	}
	// ISO-3166-2 region codes are prefixed with the ISO country code,
	// but we accept just the region code part
	if strings.Contains(correction[2], "-") {
		firstSubdivision = mmdbRecord.Country.IsoCode + "-" + firstSubdivision
	}
	if !(strings.EqualFold(correction[1], mmdbRecord.Country.IsoCode)) ||
		!(strings.EqualFold(correction[2], firstSubdivision)) ||
		!(strings.EqualFold(correction[3], mmdbRecord.City.Names["en"])) {
		diffLine := "Found a potential improvement: '%s'\n" +
			"\t\tcurrent country: '%s'\t\tsuggested country: '%s'\n" +
			"\t\tcurrent city: '%s'\t\tsuggested city: '%s'\n" +
			"\t\tcurrent region: '%s'\t\tsuggested region: '%s'\n\n"
		fmt.Printf(
			diffLine,
			networkOrIP,
			mmdbRecord.Country.IsoCode,
			correction[1],
			mmdbRecord.City.Names["en"],
			correction[3],
			firstSubdivision,
			correction[2],
		)
		return 1, nil
	}
	return 0, nil
}

func parseArgs() (string, string, error) {
	geofeedPath := flag.String(
		"geofeed-path",
		"",
		"Path to the local geofeed file to verify",
	)

	mmdbPath := flag.String(
		"mmdb-path",
		"/usr/local/share/GeoIP/GeoIP2-City.mmdb",
		"Path to MMDB file to compare geofeed file against",
	)
	flag.Parse()

	cleanGeofeedPath := filepath.Clean(*geofeedPath)
	cleanMMDBPath := filepath.Clean(*mmdbPath)

	var err error
	if cleanGeofeedPath == "." { // result of empty string, probably no arg given
		err = fmt.Errorf("'--geofeed-path' is required")
	}
	return cleanGeofeedPath, cleanMMDBPath, err
}
