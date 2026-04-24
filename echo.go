package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"time"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main23() {

	addr := flag.String("l", "127.0.0.1:5060", "My listen ip")

	username := flag.String("u", "alice", "SIP Username")
	//echoCount := flag.Int("echo", 1, "How many echos")
	// password := flag.String("p", "alice", "Password")
	flag.Parse()

	lev, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil || lev == zerolog.NoLevel {
		lev = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.StampMicro,
	}).With().Timestamp().Logger().Level(lev)

	// Setup UAC
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(*username),
		// sipgo.WithUserAgentIP(net.ParseIP(ip)),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Fail to setup user agent")
	}

	// Setup Diago server
	server := diago.NewDiago(ua,
		diago.WithTransport(diago.Transport{
			Transport: "udp",
			BindHost:  "127.0.0.1",
			BindPort:  5060,
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	log.Info().Msgf("Starting SIP server on %s", *addr)

	server.Serve(ctx, func(dlg *diago.DialogServerSession) {
		log.Info().Msg("Incoming call")

		dlg.Answer()

		// Start echo in a goroutine
		go func() {
			err := dlg.Echo()
			if err != nil {
				log.Error().Err(err).Msg("Echo failed")
			} else {
				log.Info().Msg("Echo ended gracefully")
			}
		}()

		select {
		case <-sig:
			ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
			dlg.Hangup(ctx)
			return

		case <-dlg.Context().Done():
			log.Info().Msg("Call ended")

		}

	})
}
