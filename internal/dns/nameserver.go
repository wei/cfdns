package dns

import (
	"context"
	"net"
	"time"
)

// UseDNSInternally is to change the Go program DNS only and adds 300ms between each DNS request
// because unbound rejects requests to close to each other in terms of time from the same source port
func (c *configurator) UseDNSInternally(ip net.IP) {
	c.logger.Info("using DNS address %s internally", ip.String())
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			c.internalResolverMutex.Lock()
			time.AfterFunc(300*time.Millisecond, c.internalResolverMutex.Unlock)
			return d.DialContext(ctx, "udp", net.JoinHostPort(ip.String(), "53"))
		},
	}
}
