package turn

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/pion/turn/v4"
)

func RunTurnServer(ctx context.Context) {
	publicIP := os.Getenv("BACKEND_HOST")
	if publicIP == "" {
		log.Fatalf("'public-ip' is required, BACKEND_HOST env var not set")
	}
	port := os.Getenv("TURN_SERVER_PORT")
	if port == "" {
		log.Fatalf("'port' is required, TURN_SERVER_PORT env var not set")
	}
	// Create a UDP listener to pass into pion/turn
	// pion/turn itself doesn't allocate any UDP sockets, but lets the user pass them in
	// this allows us to add logging, storage or modify inbound/outbound traffic
	udpListener, err := net.ListenPacket("udp4", "0.0.0.0:"+port)
	if err != nil {
		log.Panicf("Failed to create TURN server listener: %s", err)
	}

	users := "toto=toto"
	realm := "pion.ly"
	// Cache -users flag for easy lookup later
	// If passwords are stored they should be saved to your DB hashed using turn.GenerateAuthKey
	usersMap := map[string][]byte{}
	for _, kv := range regexp.MustCompile(`(\w+)=(\w+)`).FindAllStringSubmatch(users, -1) {
		usersMap[kv[1]] = turn.GenerateAuthKey(kv[1], realm, kv[2])
	}

	s, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		// Set AuthHandler callback
		// This is called every time a user tries to authenticate with the TURN server
		// Return the key for that user, or false when no user is found
		AuthHandler: func(username string, realm string, srcAddr net.Addr) ([]byte, bool) { // nolint: revive
			if key, ok := usersMap[username]; ok {
				return key, true
			}
			return nil, false
		},
		// PacketConnConfigs is a list of UDP Listeners and the configuration around them
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: udpListener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{
					RelayAddress: net.ParseIP(publicIP), // Claim that we are listening on IP passed by user (This should be your Public IP)
					Address:      "0.0.0.0",             // But actually be listening on every interface
				},
			},
		},
	})
	if err != nil {
		log.Panic(err)
	}

	// Block until user sends SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	if err = s.Close(); err != nil {
		log.Panic(err)
	}
}
