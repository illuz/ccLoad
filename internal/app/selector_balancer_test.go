package app

import (
	"math"
	"testing"
	"time"

	"ccLoad/internal/model"
)

func TestEffPriorityBucket_FloatEdge(t *testing.T) {
	// 模拟浮点误差：值略小于整数边界时，不应被截断到前一档。
	scaledPos := math.Nextafter(51, 0) // 50.999999999...
	pPos := scaledPos / 10
	if got := effPriorityBucket(pPos); got != 51 {
		t.Fatalf("expected bucket=51, got %d (p=%v scaled=%v)", got, pPos, pPos*10)
	}

	scaledNeg := math.Nextafter(-51, 0) // -50.999999999...
	pNeg := scaledNeg / 10
	if got := effPriorityBucket(pNeg); got != -51 {
		t.Fatalf("expected bucket=-51, got %d (p=%v scaled=%v)", got, pNeg, pNeg*10)
	}
}

func TestBalanceSamePriorityChannels_NilServerOrBalancerFallsBackToPrioritySort(t *testing.T) {
	channels := []*model.Config{
		{ID: 1, Name: "low", Priority: 1},
		{ID: 2, Name: "high", Priority: 10},
		{ID: 3, Name: "mid", Priority: 5},
	}

	var nilServer *Server
	got := nilServer.balanceSamePriorityChannels(channels, nil, dummyNow())
	if got[0].Name != "high" || got[1].Name != "mid" || got[2].Name != "low" {
		t.Fatalf("nil server fallback order = [%s %s %s]", got[0].Name, got[1].Name, got[2].Name)
	}

	serverWithoutBalancer := &Server{}
	got = serverWithoutBalancer.balanceSamePriorityChannels(channels, nil, dummyNow())
	if got[0].Name != "high" || got[1].Name != "mid" || got[2].Name != "low" {
		t.Fatalf("nil balancer fallback order = [%s %s %s]", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestSortChannelsByHealth_NilHealthCacheFallsBackWithoutPanic(t *testing.T) {
	channels := []*model.Config{
		{ID: 1, Name: "low", Priority: 1},
		{ID: 2, Name: "high", Priority: 10},
	}

	server := &Server{}
	got := server.sortChannelsByHealth(channels, nil, dummyNow())
	if got[0].Name != "high" || got[1].Name != "low" {
		t.Fatalf("fallback order = [%s %s]", got[0].Name, got[1].Name)
	}
}

func dummyNow() time.Time {
	return time.Unix(0, 0)
}
