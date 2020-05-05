package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qdm12/cloudflare-dns-server/internal/constants"
	"github.com/qdm12/cloudflare-dns-server/internal/dns"
	"github.com/qdm12/cloudflare-dns-server/internal/healthcheck"
	"github.com/qdm12/cloudflare-dns-server/internal/models"
	"github.com/qdm12/cloudflare-dns-server/internal/params"
	"github.com/qdm12/cloudflare-dns-server/internal/settings"
	"github.com/qdm12/cloudflare-dns-server/internal/splash"
	"github.com/qdm12/golibs/command"
	"github.com/qdm12/golibs/files"
	libhealthcheck "github.com/qdm12/golibs/healthcheck"
	"github.com/qdm12/golibs/logging"
	"github.com/qdm12/golibs/network"
)

func main() {
	if libhealthcheck.Mode(os.Args) {
		if err := healthcheck.Healthcheck(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	logger, err := logging.NewLogger(logging.ConsoleEncoding, logging.InfoLevel, -1)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	paramsReader := params.NewParamsReader(logger)

	fmt.Println(splash.Splash(
		paramsReader.GetVersion(),
		paramsReader.GetVcsRef(),
		paramsReader.GetBuildDate()))

	client := network.NewClient(15 * time.Second)
	// Create configurators
	fileManager := files.NewFileManager()
	dnsConf := dns.NewConfigurator(logger, client, fileManager)

	version, err := dnsConf.Version(ctx)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}
	logger.Info("Unbound version: %s", version)

	settings, err := settings.GetSettings(paramsReader)
	if err != nil {
		logger.Error(err)
		os.Exit(1)
	}
	logger.Info("Settings summary:\n" + settings.String())

	// Use plain DNS first internally to resolve github.com
	targetDNS := constants.ProviderMapping()[settings.Providers[0]]
	var targetIP net.IP
	for _, targetIP = range targetDNS.IPs {
		if settings.IPv6 && targetIP.To4() == nil {
			break
		} else if !settings.IPv6 && targetIP.To4() != nil {
			break
		}
	}
	dnsConf.UseDNSInternally(targetIP)

	streamMerger := command.NewStreamMerger()
	go streamMerger.CollectLines(ctx,
		func(line string) { logger.Info(line) },        //nolint:scopelint
		func(mergeErr error) { logger.Warn(mergeErr) }) //nolint:scopelint

	// Unbound run loop
	unboundFinished, signalUnboundFinished := context.WithCancel(context.Background())
	go func() {
		unboundCtx, unboundCancel := context.WithCancel(ctx)
		defer unboundCancel()
		for ctx.Err() == nil {
			var setupError, startError, waitError error
			unboundCtx, unboundCancel, setupError, startError, waitError = unboundRun(
				ctx, unboundCtx, unboundCancel, dnsConf, settings, streamMerger)
			switch {
			case setupError != nil:
				logger.Warn(setupError)
				const duration = time.Minute
				logger.Info("Retrying in %s", duration)
				select {
				case <-time.After(duration):
				case <-ctx.Done():
				}
			case startError != nil:
				logger.Error(startError)
				cancel()
				os.Exit(1)
			case unboundCtx.Err() == context.DeadlineExceeded:
				logger.Info("attempting planned restart")
			case waitError != nil:
				logger.Warn(waitError)
			}
		}
		logger.Info("shutting down")
		signalUnboundFinished()
	}()

	signalsCh := make(chan os.Signal, 1)
	signal.Notify(signalsCh,
		syscall.SIGINT,
		syscall.SIGTERM,
		os.Interrupt,
	)
	select {
	case signal := <-signalsCh:
		logger.Warn("Caught OS signal %s, shutting down", signal)
		cancel()
	case <-ctx.Done():
		logger.Warn("context canceled, shutting down")
	}
	<-unboundFinished.Done()
}

func unboundRun(ctx, unboundCtx context.Context, unboundCancel context.CancelFunc,
	conf dns.Configurator, settings models.Settings, streamMerger command.StreamMerger) (
	newUnboundCtx context.Context, newUnboundCancel context.CancelFunc,
	setupError, startError, waitError error) {
	if err := conf.DownloadRootHints(); err != nil {
		return unboundCtx, unboundCancel, err, nil, nil
	}
	if err := conf.DownloadRootKey(); err != nil {
		return unboundCtx, unboundCancel, err, nil, nil
	}
	if err := conf.MakeUnboundConf(settings); err != nil {
		return unboundCtx, unboundCancel, err, nil, nil
	}
	unboundCancel()
	newUnboundCtx, newUnboundCancel = context.WithTimeout(ctx, 24*time.Hour)
	stream, wait, err := conf.Start(newUnboundCtx, settings.VerbosityDetailsLevel)
	if err != nil {
		return newUnboundCtx, newUnboundCancel, nil, err, nil
	}
	go streamMerger.Merge(newUnboundCtx, stream, command.MergeName("unbound"))
	conf.UseDNSInternally(net.IP{127, 0, 0, 1})
	if settings.CheckUnbound {
		if err := conf.WaitForUnbound(); err != nil {
			return newUnboundCtx, newUnboundCancel, nil, err, nil
		}
	}
	waitError = wait()
	return newUnboundCtx, newUnboundCancel, nil, nil, waitError
}
