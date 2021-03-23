# tcp-delay-proxy
Simple TCP proxy capable of introducing delay for testing purposes for any TCP-based service.

Support is included for both static (fixed) and randomized delay.

The application is architected with dedicated, reusable objects and a lightweight CLI wrapper application.

## Static Delay
Delay can be controlled on the "upstream" (client to upstream) or "downstream" (upstream to client) independently using the corresponding `updelay/downdelay` parameters. A delay of 0 (default) short circuit and use a simpler underlying implementation.

## Randomize Deley
In addition to static delay, it is possible to randomize delay which is done using a LogNormal distribution (mu = 0, sigma = 1.0) with values scaling the specified delay. In this way the specified delay will be the median, with 50% of the sessions having a shorter delay and 50% having a longer delay.

Note that randomized delay is calcualted at the start of each session (i.e. client connection) and will thus be the same for all data sent and received within that session.

## CLI Application

### Building

Standard go build process.  Download or clone the repo and from the main directory `go build`

### Usage

```
Usage: tcp-delay-proxy [-qrv] [-d value] [-u value] listenPort upstreamAddr
 -d, --downdelay=value
       downstream delay as duration (1s, 100ms, etc.). default 0.
 -q    quiet. do not print any log info. overrides verbosity flag.
 -r, --randomizedelay
       randomize delay using lognormal distribution (mu = 0, sigma
       = 1.0) around up/down delay
 -u, --updelay=value
       upstream delay as duration (1s, 100ms, etc.). default 0.
 -v    verbosity. can be used multiple times to further increase.
 ```
 
The two required arguments are the port on which to listen and the upstream address, in that order.  The port should be a base 10 integer representing a valid and unused TCP port. The upstream address indicates the host and port to proxy and can be either IP or hostname based (e.g. `1.1.1.1:1001` or `somehost.com:80`).
 
 ## Reusing Objects Directly
 
It's alo entirely possible to instantiate objects at the various levels (Server, Session, and Pipe) directly in order to use in another application by using the objects with the `proxy` package.
 
In your code, import `github.com/wfscot/tcp-delay-proxy/proxy`.  You don't need anything from the main directory or package.
 
Note that all proxy objects require a Context to run. Cancelling that Context will cleanly tear down everything.  Furthermore, logging is implemented via a zerolog Logger instance stored in the Context via the zerolog standard Logger.WithContext() mechanism.  If the Logger is not found, logging will be disabled.  Please look to `main.go` for an example of how to do this.