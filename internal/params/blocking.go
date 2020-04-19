package params

import (
	"fmt"
	"net"
	"strings"

	libparams "github.com/qdm12/golibs/params"
)

// GetMaliciousBlocking obtains if malicious hostnames/IPs should be blocked
// from being resolved by Unbound, using the environment variable BLOCK_MALICIOUS
func (r *reader) GetMaliciousBlocking() (blocking bool, err error) {
	return r.envParams.GetOnOff("BLOCK_MALICIOUS", libparams.Default("on"))
}

// GetSurveillanceBlocking obtains if surveillance hostnames/IPs should be blocked
// from being resolved by Unbound, using the environment variable BLOCK_SURVEILLANCE
// and BLOCK_NSA for retrocompatibility
func (r *reader) GetSurveillanceBlocking() (blocking bool, err error) {
	// Retro-compatibility
	s, err := r.envParams.GetEnv("BLOCK_NSA")
	if err != nil {
		return false, err
	} else if len(s) != 0 {
		r.logger.Warn("You are using the old environment variable BLOCK_NSA, please consider changing it to BLOCK_SURVEILLANCE")
		return r.envParams.GetOnOff("BLOCK_NSA", libparams.Compulsory())
	}
	return r.envParams.GetOnOff("BLOCK_SURVEILLANCE", libparams.Default("off"))
}

// GetAdsBlocking obtains if ads hostnames/IPs should be blocked
// from being resolved by Unbound, using the environment variable BLOCK_ADS
func (r *reader) GetAdsBlocking() (blocking bool, err error) {
	return r.envParams.GetOnOff("BLOCK_ADS", libparams.Default("off"))
}

// GetUnblockedHostnames obtains a list of hostnames to unblock from block lists
// from the comma separated list for the environment variable UNBLOCK
func (r *reader) GetUnblockedHostnames() (hostnames []string, err error) {
	s, err := r.envParams.GetEnv("UNBLOCK")
	if err != nil {
		return nil, err
	}
	if len(s) == 0 {
		return nil, nil
	}
	hostnames = strings.Split(s, ",")
	for _, hostname := range hostnames {
		if !r.verifier.MatchHostname(hostname) {
			return nil, fmt.Errorf("hostname %q does not seem valid", hostname)
		}
	}
	return hostnames, nil
}

// GetBlockedHostnames obtains a list of hostnames to block from the comma
// separated list for the environment variable BLOCK_HOSTNAMES
func (r *reader) GetBlockedHostnames() (hostnames []string, err error) {
	s, err := r.envParams.GetEnv("BLOCK_HOSTNAMES")
	if err != nil {
		return nil, err
	}
	if len(s) == 0 {
		return nil, nil
	}
	hostnames = strings.Split(s, ",")
	for _, hostname := range hostnames {
		if !r.verifier.MatchHostname(hostname) {
			return nil, fmt.Errorf("hostname %q does not seem valid", hostname)
		}
	}
	return hostnames, nil
}

// GetBlockedIPs obtains a list of IP addresses or CIDR ranges to block from
// the comma separated list for the environment variable BLOCK_IPS
func (r *reader) GetBlockedIPs() (ips []string, err error) {
	s, err := r.envParams.GetEnv("BLOCK_IPS")
	if err != nil {
		return nil, err
	}
	if len(s) == 0 {
		return nil, nil
	}
	words := strings.Split(s, ",")
	for _, ip := range words {
		_, _, err = net.ParseCIDR(ip)
		if err != nil && net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("Blocked IP address %q is not a valid IP address or CIDR range", ip)
		}
		ips = append(ips, ip)
	}
	return ips, nil
}
