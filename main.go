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
	"strings"
	"time"
)

// build info, overwritten by goreleaser
var (
	version = "dev (unreleased)"
	commit  = ""
	date    = ""
)

// type config holds configuration
type config struct {
	stdout   io.Writer
	stderr   io.Writer
	host     string
	user     string
	pass     string
	insecure bool
	debug    bool
	quiet    bool
	action   string
	get      bool
	list     bool
	printver bool
	ignore   bool
	timeout  int
}

// type system describes (partial) redfish system
type system struct {
	PowerState string `json:"PowerState"`
	Actions    struct {
		ComputerSystemReset struct {
			ResetTypeRedfishAllowableValues []string `json:"ResetType@Redfish.AllowableValues"`
			Target                          string   `json:"target"`
		} `json:"#ComputerSystem.Reset"`
	} `json:"Actions"`
}

// main function
func main() {
	if err := run(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

// run parses passed arguments, builds config and runs specified function: get, list or action
func run(args []string, stdout io.Writer, stderr io.Writer) error {
	var c config
	c.stdout = stdout
	c.stderr = stderr

	// init and parse flags
	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.SetOutput(stderr)
	flags.BoolVar(&c.get, "get", false, "get current power state")
	flags.BoolVar(&c.list, "list", false, "list supported power actions")
	flags.StringVar(&c.action, "action", "", "power action to perform")
	flags.StringVar(&c.host, "host", "", "BMC address and optional port (host or host:port)")
	flags.StringVar(&c.user, "user", "", "BMC username")
	flags.StringVar(&c.pass, "pass", "", "BMC password")
	flags.BoolVar(&c.insecure, "insecure", false, "do not verify host certificate")
	flags.BoolVar(&c.debug, "debug", false, "enable printing of http response body")
	flags.BoolVar(&c.quiet, "quiet", false, "do not output any messages except errors")
	flags.BoolVar(&c.printver, "version", false, "print program version and quit")
	flags.BoolVar(&c.ignore, "ignore", false, "ignore conflicts (like power on the server which is already on)")
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
		return fmt.Errorf("missing -host argument")
	case c.user == "":
		return fmt.Errorf("missing -user name")
	case c.pass == "":
		return fmt.Errorf("missing -password")
	case c.action == "" && c.get == false && c.list == false:
		return fmt.Errorf("missing -action, -get or -list argument")
	case c.list && c.get, c.list && c.action != "", c.get && c.action != "":
		return fmt.Errorf("arguments -action, -get and -list cannot be used at the same time")
	case c.quiet && c.debug:
		return fmt.Errorf("arguments -debug and -quiet cannot be used at the same time")
	}

	// call requested function
	switch {
	case c.get:
		return get(c)
	case c.list:
		return list(c)
	case c.action != "":
		return action(c)
	}
	return fmt.Errorf("possible bug. don't know what to do")
}

// list prints out a list of supported power actions for specified hosts
// currently only hosts with single computer system in redfish systems collection are supported
func list(c config) error {
	sys, err := getSystem(c)
	if err != nil {
		return err
	}
	if !c.quiet {
		fmt.Fprintf(c.stdout, "host: %s allowed power actions:\n", c.host)
	}
	for _, val := range sys.Actions.ComputerSystemReset.ResetTypeRedfishAllowableValues {
		fmt.Fprintln(c.stdout, val)
	}
	return nil
}

// get returns current power state for specified host
// currently only hosts with single computer system in redfish systems collection are supported
func get(c config) error {
	sys, err := getSystem(c)
	if err != nil {
		return err
	}
	if !c.quiet {
		fmt.Fprintf(c.stdout, "host: %s power state: ", c.host)
	}
	fmt.Fprintln(c.stdout, sys.PowerState)
	return nil
}

// action performs selected action on specified host
// currently only hosts with single computer system in redfish systems collection are supported
func action(c config) error {
	sys, err := getSystem(c)
	if err != nil {
		return err
	}
	if !c.quiet {
		fmt.Fprintf(c.stdout, "performing %s action on host %s ...\n", c.action, c.host)
	}
	url := fmt.Sprintf("https://%s%s", c.host, sys.Actions.ComputerSystemReset.Target)
	data := fmt.Sprintf("{\"ResetType\":\"%s\"}", c.action)
	_, err = redfishPost(c, url, data)
	if err != nil {
		return err
	}
	return nil
}

// getSystem returns (partial) redfish system object for specified host or error
// currently only hosts with single computer system in redfish systems collection are supported
func getSystem(c config) (system, error) {
	url, err := getSystemURL(c)
	if err != nil {
		return system{}, err
	}
	b, err := redfishGet(c, url)
	if err != nil {
		return system{}, err
	}
	var sys system
	if err := json.Unmarshal(b, &sys); err != nil {
		return system{}, err
	}
	return sys, nil
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

// redfishGet sends http GET request to specified url and returns received reponse body or error
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
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		if c.debug {
			fmt.Fprintf(c.stderr, "response status code: %d (%s)\n", resp.StatusCode, http.StatusText(resp.StatusCode))
			fmt.Fprintln(c.stderr, "Response body:")
			fmt.Fprintf(c.stderr, string(body))
		}
		return nil, fmt.Errorf("wrong response status code - expected: 200 (OK), got: %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return body, nil
}

// redfishPost sends http POST request with json encoded data to specified url and returns received reponse body or error
func redfishPost(c config, url string, data string) ([]byte, error) {
	client := &http.Client{
		Timeout:   time.Second * time.Duration(c.timeout),
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.insecure}},
	}
	req, err := http.NewRequest("POST", url, strings.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.pass)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if c.ignore && resp.StatusCode == http.StatusConflict {
		if !c.quiet {
			fmt.Fprintln(c.stdout, "OK (ignored conflict)")
		}
		return body, nil
	}
	if (resp.StatusCode != http.StatusOK) && (resp.StatusCode != http.StatusNoContent) {
		if c.debug {
			fmt.Fprintf(c.stderr, "response status code: %d (%s)\n", resp.StatusCode, http.StatusText(resp.StatusCode))
			fmt.Fprintln(c.stderr, "Response body:")
			fmt.Fprintf(c.stderr, string(body))
		}
		return nil, fmt.Errorf("wrong response status code - expected: 200 (OK) or 204 (NoContent), got: %d (%s)", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if !c.quiet {
		fmt.Fprintln(c.stdout, "OK")
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
