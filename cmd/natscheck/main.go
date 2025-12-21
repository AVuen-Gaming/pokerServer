package main

import (
	"log"
	"server/config"
	internalnats "server/internal/nats"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	nc, js, err := internalnats.Connect(cfg)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	if info, err := js.StreamInfo("POKER_TOURNAMENT"); err == nil {
		log.Printf("stream %s subjects=%v", info.Config.Name, info.Config.Subjects)
	} else {
		log.Printf("stream info error: %v", err)
	}

	subject := "pokerServer.947927.13"
	if _, err := js.Publish(subject, []byte("ping")); err != nil {
		log.Fatalf("publish: %v", err)
	}
	log.Printf("publish to %s succeeded", subject)
}
