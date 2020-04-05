package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

// build info, overwritten by goreleaser
var (
	version = "dev (unreleased)"
	commit  = ""
	date    = ""
)

type config struct {
	stdout   io.Writer
	stderr   io.Writer
	host     string
	user     string
	pass     string
	insecure bool
	quiet    bool
	action   string
	list     bool
	printver bool
	wait     bool
	timeout  int
}

func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) error {
	var c config
	c.stdout = stdout
	c.stderr = stderr

	// init and parse flags
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&c.list, "list", false, "list supported power actions")
	flags.StringVar(&c.action, "action", "", "power action to perform")
	flags.StringVar(&c.host, "host", "", "BMC address and optional port (host or host:port)")
	flags.StringVar(&c.user, "user", "", "BMC username")
	flags.StringVar(&c.pass, "pass", "", "BMC password")
	flags.BoolVar(&c.insecure, "insecure", false, "do not verify host certificate")
	flags.BoolVar(&c.quiet, "quiet", false, "do not output any messages except errors")
	flags.BoolVar(&c.printver, "version", false, "print program version and quit")
	flags.BoolVar(&c.wait, "wait", false, "wait for action to finish")
	flags.IntVar(&c.timeout, "timeout", 30, "operation timeout in seconds")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	// verify flags
	switch {
	case len(args) < 2:
		fmt.Fprintf(stderr, "Usage of %s:\n", args[0])
		flags.PrintDefaults()
		return fmt.Errorf("no arguments provided")
	case c.printver:
		fmt.Fprintf(stdout, "redpower  version: %s (%s) build date: %s\n", version, commit, date)
		return nil
	case c.host == "":
		return fmt.Errorf("missing host argument")
	case c.user == "":
		return fmt.Errorf("missing user name")
	case c.pass == "":
		return fmt.Errorf("missing password")
	case c.list == false && c.action == "":
		return fmt.Errorf("missing action or list argument")
	case c.list == true && c.action != "":
		return fmt.Errorf("action and list arguments cannot be used at the same time")
	}

	if c.list == true {
		return list(c)
	}
	return action(c)
}

// list prints out a list of supported power actions for specified hosts
// currently only hosts with single computer system in redfish systems collection are supported
func list(c config) error {
	if !c.quiet {
		fmt.Fprintf(c.stdout, "listing available power actions on host %s ...\n", c.host)
	}
	url, err := getSystemURL(c)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.stdout, "url: %s\n", url)
	b, err := redfishGet(c, url)
	if err != nil {
		return err
	}
	var system struct {
		Actions struct {
			ComputerSystemReset struct {
				ResetTypeRedfishAllowableValues []string `json:"ResetType@Redfish.AllowableValues"`
				Target                          string   `json:"target"`
			} `json:"#ComputerSystem.Reset"`
		} `json:"Actions"`
	}
	if err := json.Unmarshal(b, &system); err != nil {
		return err
	}
	for _, a := range system.Actions.ComputerSystemReset.ResetTypeRedfishAllowableValues {
		fmt.Fprintln(c.stdout, a)
	}
	return nil
}

// action performs selected action on specified host
// currently only hosts with single computer system in redfish systems collection are supported
func action(c config) error {
	if !c.quiet {
		fmt.Fprintf(c.stdout, "performing %s action on host %s ...\n", c.action, c.host)
	}
	return nil
}

// getSystemURL returns URL for redfish computer system or error if 0 or more than 1 system is found in the systems collection
func getSystemURL(c config) (string, error) {
	url := fmt.Sprintf("https://%s/redfish/v1/Systems", c.host)
	b, err := redfishGet(c, url)
	if err != nil {
		return "", err
	}
	systems, err := parseRedfishCollection(b)
	if err != nil {
		return "", err
	}
	switch l := len(systems); {
	case l == 0:
		return "", fmt.Errorf("no systems found in the redfish systems collection")
	case l > 1:
		return "", fmt.Errorf("multiple systems found in the redfish systems collection - not supported")
	}
	return fmt.Sprintf("https://%s%s", c.host, systems[0]), nil
}

// redfishGet sends http GET request to specified url then returns received reponse body or error
func redfishGet(c config, url string) ([]byte, error) {
	client := &http.Client{
		Timeout:   time.Second * time.Duration(c.timeout),
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.insecure}},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wrong response status code - expected: 200 (OK), got: %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// parseRedfishCollection parses redfish collection and returns a list of members in a slice or error if collection cannot be parsed
func parseRedfishCollection(b []byte) ([]string, error) {
	var rc struct {
		Members []struct {
			OdataID string `json:"@odata.id"`
		} `json:"Members"`
		Count int `json:"Members@odata.count"`
	}
	if err := json.Unmarshal(b, &rc); err != nil {
		return nil, err
	}
	result := make([]string, rc.Count)
	for i, member := range rc.Members {
		result[i] = member.OdataID
	}
	return result, nil
}
