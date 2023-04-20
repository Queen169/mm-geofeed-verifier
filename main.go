// This script is meant to help verify 'bulk correction' files for submission
// to MaxMind. The files are expected to (mostly) follow the format provided by the RFC at
// https://datatracker.ietf.org/doc/rfc8805/
// Region codes without the country prefix are accepted. eg, 'NY' is allowed, along with
// 'US-NY' for the state of New York in the United States.
// Beyond verifying that the format of the data is correct, the script will also compare
// the corrections against a given MMDB, reporting on how many corrections differ from
// the contents in the database.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/maxmind/mm-geofeed-verifier/verify"
)

const version = "2.2.1"

type config struct {
	gf      string
	db      string
	isp     string
	version bool
	laxMode bool
}

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	conf, output, err := parseFlags(os.Args[0], os.Args[1:])
	if err != nil {
		fmt.Println(output)
		return err
	}

	c, diffLines, asnCounts, err := verify.ProcessGeofeed(conf.gf, conf.db, conf.isp, conf.laxMode)
	if err != nil {
		return fmt.Errorf("unable to process geofeed %s: %w", conf.gf, err)
	}

	fmt.Printf(
		strings.Join(diffLines, "\n\n")+
			"\n\nOut of %d potential corrections, %d may be different than our current mappings\n\n",
		c.Total,
		c.Differences,
	)

	// https://stackoverflow.com/a/56706305
	asNumbers := make([]uint, 0, len(asnCounts))
	for asNumber := range asnCounts {
		asNumbers = append(asNumbers, asNumber)
	}
	sort.Slice(
		asNumbers,
		func(i, j int) bool {
			return asnCounts[asNumbers[i]] > asnCounts[asNumbers[j]]
		},
	)
	for _, asNumber := range asNumbers {
		fmt.Printf("ASN: %d, count: %d\n", asNumber, asnCounts[asNumber])
	}

	return nil
}

func parseFlags(program string, args []string) (c *config, output string, err error) {
	flags := flag.NewFlagSet(program, flag.ContinueOnError)
	var buf bytes.Buffer
	flags.SetOutput(&buf)

	var conf config
	flags.StringVar(&conf.gf, "gf", "", "Path to local geofeed file to verify")
	flags.StringVar(&conf.isp, "isp", "", "Path to ISP MMDB file (optional)")
	flags.StringVar(
		&conf.db,
		"db",
		"/usr/local/share/GeoIP/GeoIP2-City.mmdb",
		"Path to MMDB file to compare geofeed file against",
	)
	flags.BoolVar(&conf.version, "V", false, "Display version")
	flags.BoolVar(
		&conf.laxMode,
		"lax",
		false,
		"Enable lax mode: geofeed's region code may be provided without country code prefix")

	err = flags.Parse(args)
	if err != nil {
		return nil, buf.String(), err
	}

	if conf.version {
		log.Printf("mm-geofeed-verifier %s", version)
		//nolint:revive // preexisting
		os.Exit(0)
	}

	if conf.gf == "" && conf.db == "" {
		flags.PrintDefaults()
		return nil, buf.String(), errors.New(
			"-gf is required and -db can not be an emptry string",
		)
	}
	if conf.gf == "" {
		flags.PrintDefaults()
		return nil, buf.String(), errors.New("-gf is required")
	}
	if conf.db == "" {
		flags.PrintDefaults()
		return nil, buf.String(), errors.New("-db is required")
	}

	return &conf, buf.String(), nil
}
