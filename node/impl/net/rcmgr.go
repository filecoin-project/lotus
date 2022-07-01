package net

import (
	"context"
	"strings"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	rcmgr "github.com/libp2p/go-libp2p-resource-manager"
	"golang.org/x/xerrors"

	"github.com/filecoin-project/lotus/api"
)

func (a *NetAPI) NetStat(ctx context.Context, scope string) (result api.NetStat, err error) {
	switch {
	case scope == "all":
		rapi, ok := a.ResourceManager.(rcmgr.ResourceManagerState)
		if !ok {
			return result, xerrors.Errorf("rexource manager does not support ResourceManagerState API")
		}

		stat := rapi.Stat()
		result.System = &stat.System
		result.Transient = &stat.Transient
		if len(stat.Services) > 0 {
			result.Services = stat.Services
		}
		if len(stat.Protocols) > 0 {
			result.Protocols = make(map[string]network.ScopeStat, len(stat.Protocols))
			for proto, stat := range stat.Protocols {
				result.Protocols[string(proto)] = stat
			}
		}
		if len(stat.Peers) > 0 {
			result.Peers = make(map[string]network.ScopeStat, len(stat.Peers))
			for p, stat := range stat.Peers {
				result.Peers[p.Pretty()] = stat
			}
		}

		return result, nil

	case scope == "system":
		err = a.ResourceManager.ViewSystem(func(s network.ResourceScope) error {
			stat := s.Stat()
			result.System = &stat
			return nil
		})
		return result, err

	case scope == "transient":
		err = a.ResourceManager.ViewTransient(func(s network.ResourceScope) error {
			stat := s.Stat()
			result.Transient = &stat
			return nil
		})
		return result, err

	case strings.HasPrefix(scope, "svc:"):
		svc := scope[4:]
		err = a.ResourceManager.ViewService(svc, func(s network.ServiceScope) error {
			stat := s.Stat()
			result.Services = map[string]network.ScopeStat{
				svc: stat,
			}
			return nil
		})
		return result, err

	case strings.HasPrefix(scope, "proto:"):
		proto := scope[6:]
		err = a.ResourceManager.ViewProtocol(protocol.ID(proto), func(s network.ProtocolScope) error {
			stat := s.Stat()
			result.Protocols = map[string]network.ScopeStat{
				proto: stat,
			}
			return nil
		})
		return result, err

	case strings.HasPrefix(scope, "peer:"):
		p := scope[5:]
		pid, err := peer.IDFromString(p)
		if err != nil {
			return result, xerrors.Errorf("invalid peer ID: %s: %w", p, err)
		}
		err = a.ResourceManager.ViewPeer(pid, func(s network.PeerScope) error {
			stat := s.Stat()
			result.Peers = map[string]network.ScopeStat{
				p: stat,
			}
			return nil
		})
		return result, err

	default:
		return result, xerrors.Errorf("invalid scope %s", scope)
	}
}

func (a *NetAPI) NetLimit(ctx context.Context, scope string) (result api.NetLimit, err error) {
	getLimit := func(s network.ResourceScope) error {
		limiter, ok := s.(rcmgr.ResourceScopeLimiter)
		if !ok {
			return xerrors.Errorf("resource scope doesn't implement ResourceScopeLimiter interface")
		}

		limit := limiter.Limit()
		switch l := limit.(type) {
		case *rcmgr.StaticLimit:
			result.Memory = l.Memory
			result.Streams = l.BaseLimit.Streams
			result.StreamsInbound = l.BaseLimit.StreamsInbound
			result.StreamsOutbound = l.BaseLimit.StreamsOutbound
			result.Conns = l.BaseLimit.Conns
			result.ConnsInbound = l.BaseLimit.ConnsInbound
			result.ConnsOutbound = l.BaseLimit.ConnsOutbound
			result.FD = l.BaseLimit.FD

		case *rcmgr.DynamicLimit:
			result.Dynamic = true
			result.MemoryFraction = l.MemoryLimit.MemoryFraction
			result.MinMemory = l.MemoryLimit.MinMemory
			result.MaxMemory = l.MemoryLimit.MaxMemory
			result.Streams = l.BaseLimit.Streams
			result.StreamsInbound = l.BaseLimit.StreamsInbound
			result.StreamsOutbound = l.BaseLimit.StreamsOutbound
			result.Conns = l.BaseLimit.Conns
			result.ConnsInbound = l.BaseLimit.ConnsInbound
			result.ConnsOutbound = l.BaseLimit.ConnsOutbound
			result.FD = l.BaseLimit.FD

		default:
			return xerrors.Errorf("unknown limit type %T", limit)
		}

		return nil
	}

	switch {
	case scope == "system":
		err = a.ResourceManager.ViewSystem(func(s network.ResourceScope) error {
			return getLimit(s)
		})
		return result, err

	case scope == "transient":
		err = a.ResourceManager.ViewTransient(func(s network.ResourceScope) error {
			return getLimit(s)
		})
		return result, err

	case strings.HasPrefix(scope, "svc:"):
		svc := scope[4:]
		err = a.ResourceManager.ViewService(svc, func(s network.ServiceScope) error {
			return getLimit(s)
		})
		return result, err

	case strings.HasPrefix(scope, "proto:"):
		proto := scope[6:]
		err = a.ResourceManager.ViewProtocol(protocol.ID(proto), func(s network.ProtocolScope) error {
			return getLimit(s)
		})
		return result, err

	case strings.HasPrefix(scope, "peer:"):
		p := scope[5:]
		pid, err := peer.IDFromString(p)
		if err != nil {
			return result, xerrors.Errorf("invalid peer ID: %s: %w", p, err)
		}
		err = a.ResourceManager.ViewPeer(pid, func(s network.PeerScope) error {
			return getLimit(s)
		})
		return result, err

	default:
		return result, xerrors.Errorf("invalid scope %s", scope)
	}
}

func (a *NetAPI) NetSetLimit(ctx context.Context, scope string, limit api.NetLimit) error {
	setLimit := func(s network.ResourceScope) error {
		limiter, ok := s.(rcmgr.ResourceScopeLimiter)
		if !ok {
			return xerrors.Errorf("resource scope doesn't implement ResourceScopeLimiter interface")
		}

		var newLimit rcmgr.Limit
		if limit.Dynamic {
			newLimit = &rcmgr.DynamicLimit{
				MemoryLimit: rcmgr.MemoryLimit{
					MemoryFraction: limit.MemoryFraction,
					MinMemory:      limit.MinMemory,
					MaxMemory:      limit.MaxMemory,
				},
				BaseLimit: rcmgr.BaseLimit{
					Streams:         limit.Streams,
					StreamsInbound:  limit.StreamsInbound,
					StreamsOutbound: limit.StreamsOutbound,
					Conns:           limit.Conns,
					ConnsInbound:    limit.ConnsInbound,
					ConnsOutbound:   limit.ConnsOutbound,
					FD:              limit.FD,
				},
			}
		} else {
			newLimit = &rcmgr.StaticLimit{
				Memory: limit.Memory,
				BaseLimit: rcmgr.BaseLimit{
					Streams:         limit.Streams,
					StreamsInbound:  limit.StreamsInbound,
					StreamsOutbound: limit.StreamsOutbound,
					Conns:           limit.Conns,
					ConnsInbound:    limit.ConnsInbound,
					ConnsOutbound:   limit.ConnsOutbound,
					FD:              limit.FD,
				},
			}
		}

		limiter.SetLimit(newLimit)
		return nil
	}

	switch {
	case scope == "system":
		err := a.ResourceManager.ViewSystem(func(s network.ResourceScope) error {
			return setLimit(s)
		})
		return err

	case scope == "transient":
		err := a.ResourceManager.ViewTransient(func(s network.ResourceScope) error {
			return setLimit(s)
		})
		return err

	case strings.HasPrefix(scope, "svc:"):
		svc := scope[4:]
		err := a.ResourceManager.ViewService(svc, func(s network.ServiceScope) error {
			return setLimit(s)
		})
		return err

	case strings.HasPrefix(scope, "proto:"):
		proto := scope[6:]
		err := a.ResourceManager.ViewProtocol(protocol.ID(proto), func(s network.ProtocolScope) error {
			return setLimit(s)
		})
		return err

	case strings.HasPrefix(scope, "peer:"):
		p := scope[5:]
		pid, err := peer.IDFromString(p)
		if err != nil {
			return xerrors.Errorf("invalid peer ID: %s: %w", p, err)
		}
		err = a.ResourceManager.ViewPeer(pid, func(s network.PeerScope) error {
			return setLimit(s)
		})
		return err

	default:
		return xerrors.Errorf("invalid scope %s", scope)
	}
}
