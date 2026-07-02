package app

import (
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"time"

	modelpkg "ccLoad/internal/model"
)

type rendezvousChannelScore struct {
	cfg    *modelpkg.Config
	score  float64
	weight int
}

func orderChannelsByWeightedRendezvous(
	channels []*modelpkg.Config,
	tokenHash string,
	keyCooldowns map[int64]map[int]time.Time,
	now time.Time,
) []*modelpkg.Config {
	if len(channels) <= 1 || tokenHash == "" {
		return channels
	}

	scored := make([]rendezvousChannelScore, 0, len(channels))
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		weight := calcEffectiveKeyCount(ch, keyCooldowns, now)
		scored = append(scored, rendezvousChannelScore{
			cfg:    ch,
			score:  weightedRendezvousScore(tokenHash, ch.ID, weight),
			weight: weight,
		})
	}
	if len(scored) <= 1 {
		result := make([]*modelpkg.Config, 0, len(scored))
		for _, item := range scored {
			result = append(result, item.cfg)
		}
		return result
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].cfg.ID < scored[j].cfg.ID
		}
		return scored[i].score > scored[j].score
	})

	result := make([]*modelpkg.Config, len(scored))
	for i, item := range scored {
		result[i] = item.cfg
	}
	return result
}

func weightedRendezvousScore(tokenHash string, channelID int64, weight int) float64 {
	if weight <= 0 {
		weight = 1
	}
	u := stableUniform01(tokenHash, channelID)
	return float64(weight) / -math.Log(u)
}

func stableUniform01(tokenHash string, channelID int64) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(tokenHash))
	_, _ = h.Write([]byte{':'})
	_, _ = h.Write([]byte(strconv.FormatInt(channelID, 10)))
	sum := h.Sum64()
	const denom = float64(^uint64(0)) + 2.0
	return (float64(sum) + 1.0) / denom
}
