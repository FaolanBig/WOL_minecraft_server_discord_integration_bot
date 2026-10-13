package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	idleShutdownAfter = 15 * time.Minute
	pollInterval      = 30 * time.Second
	operationTimeout  = 5 * time.Second
	startPollInterval = 5 * time.Second
)

type Minecraft interface {
	Players(context.Context) (int, error)
	Stop(context.Context) error
}

type Waker interface {
	Wake(context.Context) error
}

type HostControl interface {
	StartMinecraft(context.Context) error
	ScheduleShutdown(context.Context) error
	CancelShutdown(context.Context) error
}

type Manager struct {
	minecraft Minecraft
	waker     Waker
	host      HostControl

	operationMu        chan struct{}
	hostMu             chan struct{}
	startMu            chan struct{}
	stateMu            sync.Mutex
	idleSince          time.Time
	stopping           bool
	hostTimerKnown     bool
	hostTimerScheduled bool
	hostFailureLogged  bool
}

func NewManager(minecraft Minecraft, waker Waker, host HostControl) *Manager {
	return &Manager{
		minecraft:   minecraft,
		waker:       waker,
		host:        host,
		operationMu: make(chan struct{}, 1),
		hostMu:      make(chan struct{}, 1),
		startMu:     make(chan struct{}, 1),
	}
}

func (m *Manager) Start(ctx context.Context) (alreadyOnline bool, err error) {
	if err := lock(ctx, m.startMu); err != nil {
		return false, err
	}
	defer unlock(m.startMu)

	players, lastRCONErr := m.queryPlayers(ctx)
	if lastRCONErr == nil {
		m.recordPlayers(players)
		if err := m.reconcileHostShutdown(ctx, false); err != nil {
			slog.Error("could not cancel host shutdown timer while Minecraft is online", "error", err)
		}
		return true, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	hostReady := false
	lastHostErr := m.startMinecraftHost(ctx)
	if lastHostErr == nil {
		hostReady = true
	} else {
		slog.Debug("Minecraft host SSH start attempt failed; sending Wake-on-LAN", "error", lastHostErr)
		if err := m.waker.Wake(ctx); err != nil {
			return false, err
		}
		slog.Info("Wake-on-LAN packet sent")
	}

	ticker := time.NewTicker(startPollInterval)
	defer ticker.Stop()
	for {
		players, queryErr := m.queryPlayers(ctx)
		if queryErr == nil {
			m.recordPlayers(players)
			if err := m.reconcileHostShutdown(ctx, false); err != nil {
				return false, err
			}
			slog.Info("Minecraft server is reachable via RCON", "players", players)
			return false, nil
		}
		lastRCONErr = queryErr
		if !hostReady {
			if err := m.startMinecraftHost(ctx); err == nil {
				hostReady = true
				lastHostErr = nil
			} else {
				lastHostErr = err
			}
		}

		select {
		case <-ctx.Done():
			if lastHostErr != nil {
				return false, fmt.Errorf("%w; last host SSH attempt: %v", ctx.Err(), lastHostErr)
			}
			return false, fmt.Errorf("%w; last RCON check: %v", ctx.Err(), lastRCONErr)
		case <-ticker.C:
		}
	}
}

func (m *Manager) Status(ctx context.Context) (int, error) {
	players, err := m.queryPlayers(ctx)
	if err != nil {
		m.resetAfterUnreachable()
		_ = m.reconcileHostShutdown(ctx, true)
		return 0, err
	}
	m.recordPlayers(players)
	if err := m.reconcileHostShutdown(ctx, false); err != nil {
		slog.Error("could not cancel host shutdown timer while Minecraft is online", "error", err)
	}
	return players, nil
}

func (m *Manager) Monitor(ctx context.Context) {
	m.poll(ctx)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll(ctx)
		}
	}
}

func (m *Manager) poll(ctx context.Context) {
	players, err := m.queryPlayers(ctx)
	if err != nil {
		m.resetAfterUnreachable()
		_ = m.reconcileHostShutdown(ctx, true)
		if !errors.Is(err, context.Canceled) {
			slog.Debug("Minecraft RCON check failed; idle shutdown timer reset", "error", err)
		}
		return
	}

	m.stateMu.Lock()
	stopping := m.stopping
	m.stateMu.Unlock()
	if stopping {
		return
	}
	m.recordPlayers(players)
	if err := m.reconcileHostShutdown(ctx, false); err != nil {
		slog.Error("could not cancel host shutdown timer while Minecraft is online", "error", err)
	}
	if players > 0 {
		return
	}

	m.stateMu.Lock()
	idleSince := m.idleSince
	m.stateMu.Unlock()
	if time.Since(idleSince) < idleShutdownAfter {
		return
	}

	lockCtx, cancelLock := context.WithTimeout(ctx, operationTimeout)
	defer cancelLock()
	if err := lock(lockCtx, m.operationMu); err != nil {
		return
	}
	defer unlock(m.operationMu)

	checkCtx, cancelCheck := context.WithTimeout(ctx, operationTimeout)
	players, err = m.minecraft.Players(checkCtx)
	cancelCheck()
	if err != nil {
		m.resetAfterUnreachable()
		slog.Warn("Minecraft RCON recheck failed before shutdown; idle timer reset", "error", err)
		return
	}
	m.recordPlayers(players)
	if players > 0 {
		return
	}
	m.stateMu.Lock()
	idleSince = m.idleSince
	m.stateMu.Unlock()
	if idleSince.IsZero() || time.Since(idleSince) < idleShutdownAfter {
		return
	}

	stopCtx, cancelStop := context.WithTimeout(ctx, operationTimeout)
	defer cancelStop()
	if err := m.minecraft.Stop(stopCtx); err != nil {
		slog.Error("automatic Minecraft shutdown failed; will retry", "error", err)
		return
	}

	m.stateMu.Lock()
	m.idleSince = time.Time{}
	m.stopping = true
	m.stateMu.Unlock()
	slog.Info("Minecraft server stopped after being empty", "idle_duration", time.Since(idleSince).String())
}

func (m *Manager) queryPlayers(ctx context.Context) (int, error) {
	queryCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := lock(queryCtx, m.operationMu); err != nil {
		return 0, err
	}
	defer unlock(m.operationMu)
	return m.minecraft.Players(queryCtx)
}

func lock(ctx context.Context, mutex chan struct{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case mutex <- struct{}{}:
		if err := ctx.Err(); err != nil {
			unlock(mutex)
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func unlock(mutex chan struct{}) {
	<-mutex
}

func (m *Manager) recordPlayers(players int) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	if players > 0 {
		m.idleSince = time.Time{}
		return
	}
	if m.idleSince.IsZero() {
		m.idleSince = time.Now()
		slog.Info("Minecraft server is empty; idle shutdown countdown started")
	}
}

func (m *Manager) resetAfterUnreachable() {
	m.stateMu.Lock()
	m.idleSince = time.Time{}
	m.stopping = false
	m.stateMu.Unlock()
}

func (m *Manager) startMinecraftHost(ctx context.Context) error {
	hostCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := lock(hostCtx, m.hostMu); err != nil {
		return err
	}
	defer unlock(m.hostMu)

	if err := m.host.StartMinecraft(hostCtx); err != nil {
		return err
	}
	m.recordHostShutdownState(false)
	return nil
}

func (m *Manager) reconcileHostShutdown(ctx context.Context, schedule bool) error {
	hostCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := lock(hostCtx, m.hostMu); err != nil {
		return err
	}
	defer unlock(m.hostMu)

	if !schedule {
		m.stateMu.Lock()
		alreadyCancelled := m.hostTimerKnown && !m.hostTimerScheduled
		m.stateMu.Unlock()
		if alreadyCancelled {
			return nil
		}
	}

	var err error
	if schedule {
		err = m.host.ScheduleShutdown(hostCtx)
	} else {
		err = m.host.CancelShutdown(hostCtx)
	}
	if err != nil {
		m.stateMu.Lock()
		m.hostTimerKnown = false
		logFailure := !m.hostFailureLogged
		m.hostFailureLogged = true
		m.stateMu.Unlock()
		if logFailure && !errors.Is(err, context.Canceled) {
			slog.Warn("host shutdown timer control failed; will retry", "error", err)
		}
		return err
	}
	changed := m.recordHostShutdownState(schedule)
	if changed {
		if schedule {
			slog.Info("host shutdown grace timer is scheduled")
		} else {
			slog.Info("host shutdown grace timer is cancelled")
		}
	}
	return nil
}

func (m *Manager) recordHostShutdownState(scheduled bool) bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	changed := !m.hostTimerKnown || m.hostTimerScheduled != scheduled
	m.hostTimerKnown = true
	m.hostTimerScheduled = scheduled
	m.hostFailureLogged = false
	return changed
}
