package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeMinecraft struct {
	responses []playerResponse
	stops     int
}

type playerResponse struct {
	count int
	err   error
}

func (f *fakeMinecraft) Players(context.Context) (int, error) {
	if len(f.responses) == 0 {
		return 0, errors.New("unexpected player query")
	}
	response := f.responses[0]
	if len(f.responses) > 1 {
		f.responses = f.responses[1:]
	}
	return response.count, response.err
}

func (f *fakeMinecraft) Stop(context.Context) error {
	f.stops++
	return nil
}

type fakeWaker struct {
	calls int
}

func (f *fakeWaker) Wake(context.Context) error {
	f.calls++
	return nil
}

type fakeHost struct {
	starts    int
	scheduled int
	canceled  int
}

func (f *fakeHost) StartMinecraft(context.Context) error {
	f.starts++
	return nil
}

func (f *fakeHost) ScheduleShutdown(context.Context) error {
	f.scheduled++
	return nil
}

func (f *fakeHost) CancelShutdown(context.Context) error {
	f.canceled++
	return nil
}

func TestPollStopsOnlyAfterFreshEmptyCheck(t *testing.T) {
	tests := []struct {
		name          string
		responses     []playerResponse
		wantStops     int
		wantIdle      bool
		wantScheduled int
	}{
		{
			name:      "empty server past threshold",
			responses: []playerResponse{{count: 0}, {count: 0}},
			wantStops: 1,
		},
		{
			name:      "player joins before shutdown",
			responses: []playerResponse{{count: 0}, {count: 1}},
			wantIdle:  false,
		},
		{
			name:          "RCON unavailable",
			responses:     []playerResponse{{err: errors.New("connection refused")}},
			wantIdle:      false,
			wantScheduled: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minecraft := &fakeMinecraft{responses: test.responses}
			host := &fakeHost{}
			manager := NewManager(minecraft, &fakeWaker{}, host)
			manager.idleSince = time.Now().Add(-idleShutdownAfter - time.Minute)

			manager.poll(context.Background())

			if minecraft.stops != test.wantStops {
				t.Errorf("Stop calls = %d, want %d", minecraft.stops, test.wantStops)
			}
			if gotIdle := !manager.idleSince.IsZero(); gotIdle != test.wantIdle {
				t.Errorf("idle timer active = %v, want %v", gotIdle, test.wantIdle)
			}
			if host.scheduled != test.wantScheduled {
				t.Errorf("host shutdown schedules = %d, want %d", host.scheduled, test.wantScheduled)
			}
		})
	}
}

func TestHostPoweroffTimerStartsAfterMinecraftStops(t *testing.T) {
	minecraft := &fakeMinecraft{responses: []playerResponse{
		{count: 0},
		{count: 0},
		{count: 0},
		{err: errors.New("connection refused")},
	}}
	host := &fakeHost{}
	manager := NewManager(minecraft, &fakeWaker{}, host)
	manager.idleSince = time.Now().Add(-idleShutdownAfter - time.Minute)

	manager.poll(context.Background())
	manager.poll(context.Background())
	manager.poll(context.Background())

	if minecraft.stops != 1 {
		t.Errorf("Minecraft stop calls = %d, want 1", minecraft.stops)
	}
	if host.scheduled != 1 {
		t.Errorf("host shutdown schedules = %d, want 1 after RCON confirms Minecraft stopped", host.scheduled)
	}
}

func TestStartStartsMinecraftServiceWhenHostIsReachable(t *testing.T) {
	minecraft := &fakeMinecraft{responses: []playerResponse{
		{err: errors.New("RCON unavailable")},
		{count: 0},
	}}
	waker := &fakeWaker{}
	host := &fakeHost{}
	manager := NewManager(minecraft, waker, host)

	alreadyOnline, err := manager.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if alreadyOnline {
		t.Fatal("Start() reported an offline server as already online")
	}
	if host.starts != 1 {
		t.Errorf("host service start calls = %d, want 1", host.starts)
	}
	if waker.calls != 0 {
		t.Errorf("Wake-on-LAN calls = %d, want 0 for an SSH-reachable host", waker.calls)
	}
}
