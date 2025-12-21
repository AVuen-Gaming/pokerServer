package nats

import (
	"errors"
	"fmt"
	"server/config"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

func Connect(cfg *config.Config) (*nats.Conn, nats.JetStreamContext, error) {
	address := fmt.Sprintf("nats://%s:%s@%s:%d", cfg.NATS.Username, cfg.NATS.Password, cfg.NATS.Host, cfg.NATS.Port)
	opts := []nats.Option{
		nats.Name("PokerServer"),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			fmt.Println("Reconnected to NATS!")
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			fmt.Printf("Disconnected from NATS: %v\n", err)
		}),
	}

	nc, err := nats.Connect(address, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      "POKER_TOURNAMENT",
		Subjects:  []string{"pokerServer.>", "pokerClient.>"},
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
	})
	if err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return nil, nil, fmt.Errorf("failed to add stream: %w", err)
	}

	return nc, js, nil
}

func ConfigureStream(js nats.JetStreamContext, streamCfg *config.StreamConfig) error {
	if streamCfg == nil || streamCfg.Name == "" {
		return nil
	}
	desiredSubjects := uniqueSubjects(streamCfg.Subjects)
	config := &nats.StreamConfig{
		Name:      streamCfg.Name,
		Subjects:  desiredSubjects,
		Retention: nats.LimitsPolicy,
		MaxAge:    1 * time.Hour,
	}
	_, err := js.AddStream(config)
	if err == nil {
		return nil
	}
	if errors.Is(err, nats.ErrStreamNameAlreadyInUse) || strings.Contains(err.Error(), "subjects overlap") {
		info, infoErr := js.StreamInfo(streamCfg.Name)
		if infoErr != nil {
			if errors.Is(infoErr, nats.ErrStreamNotFound) {
				return nil
			}
			return fmt.Errorf("failed to inspect existing stream %s: %w", streamCfg.Name, infoErr)
		}
		existing := make(map[string]struct{}, len(info.Config.Subjects))
		for _, subj := range info.Config.Subjects {
			existing[subj] = struct{}{}
		}
		updatedSubjects := append([]string{}, info.Config.Subjects...)
		updated := false
		for _, subj := range desiredSubjects {
			if _, ok := existing[subj]; ok || subj == "" {
				continue
			}
			updatedSubjects = append(updatedSubjects, subj)
			existing[subj] = struct{}{}
			updated = true
		}
		if updated {
			newCfg := info.Config
			newCfg.Subjects = updatedSubjects
			if _, err := js.UpdateStream(&newCfg); err != nil {
				return fmt.Errorf("failed to update existing stream %s: %w", streamCfg.Name, err)
			}
		}
		return nil
	}
	return fmt.Errorf("failed to add stream: %w", err)
}

func uniqueSubjects(subjects []string) []string {
	if len(subjects) == 0 {
		return nil
	}
	uniq := make(map[string]struct{})
	ordered := make([]string, 0, len(subjects))
	for _, subj := range subjects {
		subj = strings.TrimSpace(subj)
		if subj == "" {
			continue
		}
		if _, ok := uniq[subj]; ok {
			continue
		}
		uniq[subj] = struct{}{}
		ordered = append(ordered, subj)
	}
	return ordered
}
