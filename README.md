# redpower
Server power management using Redfish API

To get current power state:
```
./redpower -host HOST -user USER -pass PASSWORD -get
```

To list supported power actions for specified host:
```
./redpower -host HOST -user USER -pass PASSWORD -list
```

To perform specified action on a host:
```
./redpower -host HOST -user USER -pass PASSWORD -action ACTION
```
:exclamation: ACTION is one of the supported actions returned by -list command (case sensitive!)


Other useful arguments: *-quiet* to limit verbosity, *-insecure* to allow self-signed and invalid certificates, *-ignore* to ignore conflicts (for example when trying to power on a server which is already on). Full list below:

```
./redpower -version
redpower  version: 0.3.0 (f01caf46a505d0be8af80515a855292eb0e2131f) build date: 2020-04-05T18:52:38Z

./redpower -help   
Usage of ./redpower:
  -action string
        power action to perform
  -debug
        enable printing of http response body
  -get
        get current power state
  -host string
        BMC address and optional port (host or host:port)
  -ignore
        ignore conflicts (like power on the server which is already on)
  -insecure
        do not verify host certificate
  -list
        list supported power actions
  -pass string
        BMC password
  -quiet
        do not output any messages except errors
  -timeout int
        operation timeout in seconds (default 30)
  -user string
        BMC username
  -version
        print program version and quit
 ```       
