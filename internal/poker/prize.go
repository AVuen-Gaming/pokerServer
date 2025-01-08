package poker

func GeneratePrizeList(numParticipants int, totalPot float32) []map[string]interface{} {
	var prizeList []map[string]interface{}
	var distribution []float32

	switch {
	case numParticipants < 5:
		distribution = []float32{1.0}
	case numParticipants <= 10:
		distribution = []float32{0.5, 0.3, 0.2}
	case numParticipants <= 50:
		distribution = []float32{0.4, 0.25, 0.15, 0.1, 0.05, 0.05}
	case numParticipants <= 100:
		distribution = []float32{0.35, 0.2, 0.15, 0.1, 0.05, 0.05, 0.05, 0.05}
	case numParticipants <= 200:
		distribution = []float32{0.3, 0.2, 0.15, 0.1, 0.05, 0.05, 0.05, 0.05, 0.025, 0.025}
	case numParticipants <= 400:
		distribution = []float32{0.25, 0.2, 0.15, 0.1, 0.075, 0.05, 0.05, 0.05, 0.025, 0.025, 0.025, 0.025}
	case numParticipants > 400:
		distribution = []float32{0.2, 0.18, 0.15, 0.1, 0.08, 0.05, 0.05, 0.05, 0.025, 0.025, 0.015, 0.015, 0.015, 0.015, 0.01, 0.01}
	}

	for i, percentage := range distribution {
		if i >= numParticipants {
			break
		}
		prizeList = append(prizeList, map[string]interface{}{
			"position":       i + 1,
			"prize":          totalPot * percentage,
			"currency":       "usdt",
			"wallet_address": "",
		})
	}

	return prizeList
}
